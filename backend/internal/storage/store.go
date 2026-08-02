package storage

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/dduchieu793/go-dashboard/backend/internal/artifact"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("record not found")

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if path != ":memory:" && path != "file::memory:?cache=shared" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA foreign_keys = ON; PRAGMA busy_timeout = 5000;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure database: %w", err)
	}
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (store *Store) Close() error { return store.db.Close() }

func (store *Store) migrate(ctx context.Context) error {
	if _, err := store.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		var count int
		if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version=?`, name).Scan(&count); err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		contents, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		tx, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, string(contents)); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)`, name, time.Now().UnixMilli()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func (store *Store) SaveDefinition(ctx context.Context, definition workflow.Definition) error {
	encoded, err := json.Marshal(definition)
	if err != nil {
		return fmt.Errorf("encode workflow definition: %w", err)
	}
	_, err = store.db.ExecContext(ctx, `
		INSERT INTO workflow_definitions (id, version, name, enabled, definition_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id, version) DO UPDATE SET name=excluded.name, enabled=excluded.enabled, definition_json=excluded.definition_json`,
		definition.ID, definition.Version, definition.Name, definition.Enabled, string(encoded), time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("save workflow definition: %w", err)
	}
	return nil
}

func (store *Store) CreateRun(ctx context.Context, run workflow.Run) error {
	requestJSON, err := json.Marshal(run.Request)
	if err != nil {
		return fmt.Errorf("encode normalized request: %w", err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin create run: %w", err)
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `INSERT INTO workflow_runs
		(id, workflow_id, workflow_version, request_id, request_json, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, run.ID, run.WorkflowID, run.WorkflowVersion, run.Request.ID, string(requestJSON), run.Status, run.CreatedAt.UnixMilli())
	if err != nil {
		return fmt.Errorf("insert workflow run: %w", err)
	}
	for _, step := range run.Steps {
		_, err = tx.ExecContext(ctx, `INSERT INTO step_runs
			(id, workflow_run_id, step_id, capability, model_profile, status, attempt, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, step.ID, step.WorkflowRunID, step.StepID, step.Capability,
			step.ModelProfile, step.Status, step.Attempt, step.CreatedAt.UnixMilli())
		if err != nil {
			return fmt.Errorf("insert step run %q: %w", step.StepID, err)
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO execution_events (workflow_run_id, status, created_at) VALUES (?, ?, ?)`,
		run.ID, run.Status, run.CreatedAt.UnixMilli()); err != nil {
		return fmt.Errorf("insert run event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit create run: %w", err)
	}
	return nil
}

func (store *Store) UpdateRun(ctx context.Context, run workflow.Run) error {
	query := `UPDATE workflow_runs SET status=?, current_step_id=?, final_artifact_id=?,
		error_code=?, error_message=?, started_at=?, completed_at=? WHERE id=?`
	if run.Status != workflow.RunCancelled {
		query += ` AND status != 'cancelled'`
	}
	result, err := store.db.ExecContext(ctx, query, run.Status, run.CurrentStepID, run.FinalArtifactID, run.ErrorCode,
		run.ErrorMessage, nullableTime(run.StartedAt), nullableTime(run.CompletedAt), run.ID)
	if err != nil {
		return fmt.Errorf("update workflow run: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		if existing, getErr := store.GetRun(ctx, run.ID); getErr == nil && existing.Status == workflow.RunCancelled {
			return workflow.ErrRunCancelled
		}
		return ErrNotFound
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO execution_events (workflow_run_id, status, details_json, created_at)
		VALUES (?, ?, ?, ?)`, run.ID, run.Status, `{"current_step_id":"`+run.CurrentStepID+`"}`, time.Now().UnixMilli())
	return err
}

func (store *Store) UpdateStep(ctx context.Context, step workflow.StepRun) error {
	result, err := store.db.ExecContext(ctx, `UPDATE step_runs SET model=?, status=?, attempt=?, input_json=?, output_json=?,
		error_code=?, error_message=?, started_at=?, completed_at=? WHERE id=? AND EXISTS (
			SELECT 1 FROM workflow_runs WHERE id=step_runs.workflow_run_id AND status != 'cancelled'
		)`, step.Model, step.Status, step.Attempt,
		nullableJSON(step.Input), nullableJSON(step.Output), step.ErrorCode, step.ErrorMessage, nullableTime(step.StartedAt),
		nullableTime(step.CompletedAt), step.ID)
	if err != nil {
		return fmt.Errorf("update step run: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return workflow.ErrRunCancelled
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO execution_events (workflow_run_id, step_run_id, status, created_at)
		VALUES (?, ?, ?, ?)`, step.WorkflowRunID, step.ID, step.Status, time.Now().UnixMilli())
	return err
}

func (store *Store) CreateArtifact(ctx context.Context, value artifact.Artifact) error {
	result, err := store.db.ExecContext(ctx, `INSERT INTO artifacts
		(id, workflow_run_id, step_run_id, type, content_json, model, prompt_version, created_at)
		SELECT ?, ?, ?, ?, ?, ?, ?, ? WHERE EXISTS (
			SELECT 1 FROM workflow_runs WHERE id=? AND status != 'cancelled'
		)`, value.ID, value.WorkflowRunID, value.StepRunID, value.Type, string(value.Content), value.Model,
		value.PromptVersion, value.CreatedAt.UnixMilli(), value.WorkflowRunID)
	if err != nil {
		return fmt.Errorf("insert artifact: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return workflow.ErrRunCancelled
	}
	return nil
}

func (store *Store) CompleteStep(ctx context.Context, step workflow.StepRun, value artifact.Artifact) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin complete workflow step: %w", err)
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE step_runs SET model=?, status=?, attempt=?, input_json=?, output_json=?,
		error_code=?, error_message=?, started_at=?, completed_at=? WHERE id=? AND EXISTS (
			SELECT 1 FROM workflow_runs WHERE id=step_runs.workflow_run_id AND status != 'cancelled'
		)`, step.Model, step.Status, step.Attempt, nullableJSON(step.Input), nullableJSON(step.Output), step.ErrorCode,
		step.ErrorMessage, nullableTime(step.StartedAt), nullableTime(step.CompletedAt), step.ID)
	if err != nil {
		return fmt.Errorf("complete workflow step: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return workflow.ErrRunCancelled
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO artifacts
		(id, workflow_run_id, step_run_id, type, content_json, model, prompt_version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, value.ID, value.WorkflowRunID, value.StepRunID, value.Type,
		string(value.Content), value.Model, value.PromptVersion, value.CreatedAt.UnixMilli()); err != nil {
		return fmt.Errorf("insert completed step artifact: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO execution_events (workflow_run_id, step_run_id, status, created_at)
		VALUES (?, ?, ?, ?)`, step.WorkflowRunID, step.ID, step.Status, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("record completed workflow step: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit completed workflow step: %w", err)
	}
	return nil
}

func (store *Store) GetRun(ctx context.Context, id string) (workflow.Run, error) {
	run := workflow.Run{Steps: make([]workflow.StepRun, 0), Artifacts: make([]artifact.Artifact, 0)}
	var requestJSON string
	var startedAt, completedAt sql.NullInt64
	var createdAt int64
	err := store.db.QueryRowContext(ctx, `SELECT id, workflow_id, workflow_version, request_json, status,
		current_step_id, final_artifact_id, error_code, error_message, started_at, completed_at, created_at
		FROM workflow_runs WHERE id=?`, id).Scan(&run.ID, &run.WorkflowID, &run.WorkflowVersion, &requestJSON, &run.Status,
		&run.CurrentStepID, &run.FinalArtifactID, &run.ErrorCode, &run.ErrorMessage, &startedAt, &completedAt, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return workflow.Run{}, ErrNotFound
	}
	if err != nil {
		return workflow.Run{}, fmt.Errorf("get workflow run: %w", err)
	}
	run.CreatedAt = time.UnixMilli(createdAt)
	run.StartedAt = fromNullableTime(startedAt)
	run.CompletedAt = fromNullableTime(completedAt)
	if err := json.Unmarshal([]byte(requestJSON), &run.Request); err != nil {
		return workflow.Run{}, fmt.Errorf("decode normalized request: %w", err)
	}
	if err := store.loadSteps(ctx, &run); err != nil {
		return workflow.Run{}, err
	}
	if err := store.loadArtifacts(ctx, &run); err != nil {
		return workflow.Run{}, err
	}
	return run, nil
}

func (store *Store) FindRunByRequestID(ctx context.Context, requestID string) (workflow.Run, bool, error) {
	var id string
	err := store.db.QueryRowContext(ctx, `SELECT id FROM workflow_runs WHERE request_id=?`, requestID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return workflow.Run{}, false, nil
	}
	if err != nil {
		return workflow.Run{}, false, fmt.Errorf("find workflow run by request: %w", err)
	}
	run, err := store.GetRun(ctx, id)
	return run, true, err
}

func (store *Store) ClaimNextPendingRun(ctx context.Context) (workflow.Run, bool, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return workflow.Run{}, false, fmt.Errorf("begin claim workflow run: %w", err)
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id FROM workflow_runs WHERE status=? ORDER BY created_at LIMIT 1`,
		workflow.RunPending).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return workflow.Run{}, false, nil
	}
	if err != nil {
		return workflow.Run{}, false, fmt.Errorf("select pending workflow run: %w", err)
	}
	now := time.Now()
	result, err := tx.ExecContext(ctx, `UPDATE workflow_runs SET status=?, started_at=?, completed_at=NULL
		WHERE id=? AND status=?`, workflow.RunRunning, now.UnixMilli(), id, workflow.RunPending)
	if err != nil {
		return workflow.Run{}, false, fmt.Errorf("claim workflow run: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return workflow.Run{}, false, nil
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO execution_events (workflow_run_id, status, created_at) VALUES (?, ?, ?)`,
		id, workflow.RunRunning, now.UnixMilli()); err != nil {
		return workflow.Run{}, false, fmt.Errorf("record claimed workflow run: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return workflow.Run{}, false, fmt.Errorf("commit claimed workflow run: %w", err)
	}
	run, err := store.GetRun(ctx, id)
	return run, true, err
}

func (store *Store) ResetRunForRetry(ctx context.Context, id string) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workflow retry: %w", err)
	}
	defer tx.Rollback()
	var status workflow.RunStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM workflow_runs WHERE id=?`, id).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("load workflow retry status: %w", err)
	}
	if status != workflow.RunFailed {
		return workflow.ErrRunNotRetryable
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM artifacts WHERE workflow_run_id=? AND step_run_id IN (
		SELECT id FROM step_runs WHERE workflow_run_id=? AND status != ?
	)`, id, id, workflow.StepCompleted); err != nil {
		return fmt.Errorf("remove retry artifacts: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE step_runs SET status=?, model='', input_json=NULL, output_json=NULL,
		error_code='', error_message='', started_at=NULL, completed_at=NULL
		WHERE workflow_run_id=? AND status != ?`, workflow.StepPending, id, workflow.StepCompleted); err != nil {
		return fmt.Errorf("reset retry steps: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE workflow_runs SET status=?, current_step_id='', final_artifact_id='',
		error_code='', error_message='', started_at=NULL, completed_at=NULL WHERE id=?`, workflow.RunPending, id); err != nil {
		return fmt.Errorf("reset workflow retry: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO execution_events (workflow_run_id, status, details_json, created_at)
		VALUES (?, ?, ?, ?)`, id, workflow.RunPending, `{"reason":"manual_retry"}`, time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("record workflow retry: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workflow retry: %w", err)
	}
	return nil
}

func (store *Store) CancelRun(ctx context.Context, id string) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workflow cancellation: %w", err)
	}
	defer tx.Rollback()
	var status workflow.RunStatus
	if err := tx.QueryRowContext(ctx, `SELECT status FROM workflow_runs WHERE id=?`, id).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("load workflow cancellation status: %w", err)
	}
	if status != workflow.RunPending && status != workflow.RunRunning && status != workflow.RunWaitingApproval {
		return workflow.ErrRunNotCancellable
	}
	now := time.Now().UnixMilli()
	if _, err = tx.ExecContext(ctx, `UPDATE step_runs SET status=?, error_code='cancelled',
		error_message='Cancelled by user.', completed_at=? WHERE workflow_run_id=? AND status IN (?, ?)`,
		workflow.StepCancelled, now, id, workflow.StepPending, workflow.StepRunning); err != nil {
		return fmt.Errorf("cancel workflow steps: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `UPDATE workflow_runs SET status=?, current_step_id='', error_code='cancelled',
		error_message='Cancelled by user.', completed_at=? WHERE id=?`, workflow.RunCancelled, now, id); err != nil {
		return fmt.Errorf("cancel workflow run: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO execution_events (workflow_run_id, status, details_json, created_at)
		VALUES (?, ?, ?, ?)`, id, workflow.RunCancelled, `{"reason":"user_cancelled"}`, now); err != nil {
		return fmt.Errorf("record workflow cancellation: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workflow cancellation: %w", err)
	}
	return nil
}

func (store *Store) ListRuns(ctx context.Context, limit int) ([]workflow.Run, error) {
	if limit < 1 || limit > 100 {
		limit = 25
	}
	rows, err := store.db.QueryContext(ctx, `SELECT id FROM workflow_runs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list workflow runs: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	runs := make([]workflow.Run, 0, len(ids))
	for _, id := range ids {
		run, err := store.GetRun(ctx, id)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func (store *Store) PrepareRecovery(ctx context.Context) ([]workflow.Run, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT id FROM workflow_runs WHERE status IN (?, ?) ORDER BY created_at`,
		workflow.RunPending, workflow.RunRunning)
	if err != nil {
		return nil, fmt.Errorf("list recoverable runs: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		tx, err := store.db.BeginTx(ctx, nil)
		if err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM artifacts WHERE workflow_run_id=? AND step_run_id IN (
			SELECT id FROM step_runs WHERE workflow_run_id=? AND status != ?
		)`, id, id, workflow.StepCompleted); err != nil {
			tx.Rollback()
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE step_runs SET status=?, model='', input_json=NULL,
			output_json=NULL, error_code='', error_message='', started_at=NULL, completed_at=NULL
			WHERE workflow_run_id=? AND status != ?`, workflow.StepPending, id, workflow.StepCompleted); err != nil {
			tx.Rollback()
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, `UPDATE workflow_runs SET status=?, current_step_id='', final_artifact_id='',
			error_code='', error_message='', started_at=NULL, completed_at=NULL WHERE id=?`, workflow.RunPending, id); err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
	}
	runs := make([]workflow.Run, 0, len(ids))
	for _, id := range ids {
		run, err := store.GetRun(ctx, id)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

func (store *Store) loadSteps(ctx context.Context, run *workflow.Run) error {
	rows, err := store.db.QueryContext(ctx, `SELECT id, workflow_run_id, step_id, capability, model_profile, model,
		status, attempt, input_json, output_json, error_code, error_message, started_at, completed_at, created_at
		FROM step_runs WHERE workflow_run_id=? ORDER BY created_at, rowid`, run.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var step workflow.StepRun
		var input, output sql.NullString
		var started, completed sql.NullInt64
		var created int64
		if err := rows.Scan(&step.ID, &step.WorkflowRunID, &step.StepID, &step.Capability, &step.ModelProfile, &step.Model,
			&step.Status, &step.Attempt, &input, &output, &step.ErrorCode, &step.ErrorMessage, &started, &completed, &created); err != nil {
			return err
		}
		step.Input, step.Output = rawJSON(input), rawJSON(output)
		step.StartedAt, step.CompletedAt = fromNullableTime(started), fromNullableTime(completed)
		step.CreatedAt = time.UnixMilli(created)
		run.Steps = append(run.Steps, step)
	}
	return rows.Err()
}

func (store *Store) loadArtifacts(ctx context.Context, run *workflow.Run) error {
	rows, err := store.db.QueryContext(ctx, `SELECT id, workflow_run_id, step_run_id, type, content_json, model,
		prompt_version, created_at FROM artifacts WHERE workflow_run_id=? ORDER BY created_at, rowid`, run.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var value artifact.Artifact
		var content string
		var created int64
		if err := rows.Scan(&value.ID, &value.WorkflowRunID, &value.StepRunID, &value.Type, &content, &value.Model,
			&value.PromptVersion, &created); err != nil {
			return err
		}
		value.Content = json.RawMessage(content)
		value.CreatedAt = time.UnixMilli(created)
		run.Artifacts = append(run.Artifacts, value)
	}
	return rows.Err()
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UnixMilli()
}

func fromNullableTime(value sql.NullInt64) *time.Time {
	if !value.Valid {
		return nil
	}
	parsed := time.UnixMilli(value.Int64)
	return &parsed
}

func nullableJSON(value json.RawMessage) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}

func rawJSON(value sql.NullString) json.RawMessage {
	if !value.Valid {
		return nil
	}
	return json.RawMessage(value.String)
}
