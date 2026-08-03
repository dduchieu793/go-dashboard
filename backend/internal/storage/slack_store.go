package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dduchieu793/go-dashboard/backend/internal/slack"
)

func (store *Store) ClaimSlackEvent(ctx context.Context, eventID string, receivedAt time.Time) (bool, error) {
	now := time.Now().UTC()
	result, err := store.db.ExecContext(ctx, `INSERT INTO slack_events(event_id, status, received_at, claimed_at)
		VALUES (?, 'processing', ?, ?) ON CONFLICT(event_id) DO NOTHING`, eventID, receivedAt.UnixMilli(), now.UnixMilli())
	if err != nil {
		return false, fmt.Errorf("claim Slack event: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil || rows == 1 {
		return rows == 1, err
	}
	var status string
	var claimedAt int64
	if err := store.db.QueryRowContext(ctx, `SELECT status, claimed_at FROM slack_events WHERE event_id=?`, eventID).Scan(&status, &claimedAt); err != nil {
		return false, fmt.Errorf("read claimed Slack event: %w", err)
	}
	if status == "processed" {
		return false, nil
	}
	if time.Since(time.UnixMilli(claimedAt)) <= 5*time.Minute {
		return false, slack.ErrEventInProgress
	}
	result, err = store.db.ExecContext(ctx, `UPDATE slack_events SET received_at=?, claimed_at=?
		WHERE event_id=? AND status='processing' AND claimed_at=?`, receivedAt.UnixMilli(), now.UnixMilli(), eventID, claimedAt)
	if err != nil {
		return false, fmt.Errorf("reclaim stale Slack event: %w", err)
	}
	rows, _ = result.RowsAffected()
	if rows == 0 {
		return false, slack.ErrEventInProgress
	}
	return true, nil
}

func (store *Store) CompleteSlackEvent(ctx context.Context, eventID string, processedAt time.Time) error {
	_, err := store.db.ExecContext(ctx, `UPDATE slack_events SET status='processed', processed_at=? WHERE event_id=?`, processedAt.UnixMilli(), eventID)
	if err != nil {
		return fmt.Errorf("complete Slack event: %w", err)
	}
	return nil
}

func (store *Store) ReleaseSlackEvent(ctx context.Context, eventID string) error {
	_, err := store.db.ExecContext(ctx, `DELETE FROM slack_events WHERE event_id=? AND status='processing'`, eventID)
	return err
}

func (store *Store) EnsureSlackThread(ctx context.Context, identity slack.ThreadIdentity, status slack.ThreadSyncStatus) (slack.Thread, bool, error) {
	if err := identity.Validate(); err != nil {
		return slack.Thread{}, false, err
	}
	now := time.Now().UTC()
	result, err := store.db.ExecContext(ctx, `INSERT INTO slack_threads
		(id, workspace_id, channel_id, thread_ts, sync_status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(workspace_id, channel_id, thread_ts) DO NOTHING`,
		identity.LocalID(), identity.WorkspaceID, identity.ChannelID, identity.ThreadTS, status, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		return slack.Thread{}, false, fmt.Errorf("ensure Slack thread: %w", err)
	}
	rows, _ := result.RowsAffected()
	thread, err := store.GetSlackThreadByIdentity(ctx, identity)
	return thread, rows == 1, err
}

func (store *Store) GetSlackThreadByIdentity(ctx context.Context, identity slack.ThreadIdentity) (slack.Thread, error) {
	return scanSlackThread(store.db.QueryRowContext(ctx, slackThreadSelect+` WHERE workspace_id=? AND channel_id=? AND thread_ts=?`,
		identity.WorkspaceID, identity.ChannelID, identity.ThreadTS))
}

func (store *Store) GetSlackThread(ctx context.Context, id string) (slack.Thread, error) {
	return scanSlackThread(store.db.QueryRowContext(ctx, slackThreadSelect+` WHERE id=?`, id))
}

func (store *Store) ListSlackThreads(ctx context.Context, limit int) ([]slack.Thread, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := store.db.QueryContext(ctx, slackThreadSelect+` ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list Slack threads: %w", err)
	}
	defer rows.Close()
	threads := make([]slack.Thread, 0)
	for rows.Next() {
		thread, err := scanSlackThread(rows)
		if err != nil {
			return nil, err
		}
		threads = append(threads, thread)
	}
	return threads, rows.Err()
}

func (store *Store) UpdateSlackThreadStatus(ctx context.Context, id string, status slack.ThreadSyncStatus) error {
	now := time.Now().UTC()
	result, err := store.db.ExecContext(ctx, `UPDATE slack_threads SET sync_status=?, updated_at=? WHERE id=?`, status, now.UnixMilli(), id)
	if err != nil {
		return fmt.Errorf("update Slack thread status: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return slack.ErrNotFound
	}
	return nil
}

func (store *Store) ApplySlackMessages(ctx context.Context, threadID string, inputs []slack.MessageInput, status slack.ThreadSyncStatus) (slack.Thread, bool, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return slack.Thread{}, false, fmt.Errorf("begin Slack message update: %w", err)
	}
	defer tx.Rollback()
	thread, err := scanSlackThread(tx.QueryRowContext(ctx, slackThreadSelect+` WHERE id=?`, threadID))
	if err != nil {
		return slack.Thread{}, false, err
	}
	now := time.Now().UTC()
	changed := false
	lastMessageTS := thread.LastMessageTS
	parentMessageID := thread.ParentMessageID
	for _, input := range inputs {
		if input.WorkspaceID != thread.WorkspaceID || input.ChannelID != thread.ChannelID || input.ThreadTimestamp != thread.ThreadTS || input.SlackTimestamp == "" {
			return slack.Thread{}, false, slack.ErrInvalidIdentity
		}
		messageID := input.Identity().LocalID()
		contentHash := input.ContentHash()
		var existingHash, existingAuthor string
		var existingDeleted bool
		queryErr := tx.QueryRowContext(ctx, `SELECT content_hash, author_id, is_deleted FROM slack_messages
			WHERE workspace_id=? AND channel_id=? AND slack_message_ts=?`, input.WorkspaceID, input.ChannelID, input.SlackTimestamp).
			Scan(&existingHash, &existingAuthor, &existingDeleted)
		materialChange := errors.Is(queryErr, sql.ErrNoRows) || existingHash != contentHash || existingDeleted != input.IsDeleted || existingAuthor != input.AuthorID
		if queryErr != nil && !errors.Is(queryErr, sql.ErrNoRows) {
			return slack.Thread{}, false, fmt.Errorf("read Slack message: %w", queryErr)
		}
		createdAt := slack.SlackTime(input.SlackTimestamp)
		if createdAt.IsZero() {
			createdAt = now
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO slack_messages
			(id, thread_id, workspace_id, channel_id, slack_message_ts, thread_ts, author_id, text, content_hash,
			 is_parent, is_deleted, edited_at, deleted_at, slack_created_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(workspace_id, channel_id, slack_message_ts) DO UPDATE SET
			 thread_id=excluded.thread_id, thread_ts=excluded.thread_ts, author_id=excluded.author_id,
			 text=excluded.text, content_hash=excluded.content_hash, is_parent=excluded.is_parent,
			 is_deleted=excluded.is_deleted, edited_at=excluded.edited_at, deleted_at=excluded.deleted_at,
			 updated_at=excluded.updated_at`, messageID, thread.ID, input.WorkspaceID, input.ChannelID,
			input.SlackTimestamp, input.ThreadTimestamp, input.AuthorID, input.Text, contentHash,
			input.SlackTimestamp == input.ThreadTimestamp, input.IsDeleted, nullableTime(input.EditedAt), nullableTime(input.DeletedAt),
			createdAt.UnixMilli(), now.UnixMilli(), now.UnixMilli())
		if err != nil {
			return slack.Thread{}, false, fmt.Errorf("upsert Slack message: %w", err)
		}
		if input.AttachmentsKnown {
			attachmentChanged, attachmentErr := applySlackAttachments(ctx, tx, thread, messageID, input.Attachments, now)
			if attachmentErr != nil {
				return slack.Thread{}, false, attachmentErr
			}
			materialChange = materialChange || attachmentChanged
		}
		changed = changed || materialChange
		if input.SlackTimestamp > lastMessageTS {
			lastMessageTS = input.SlackTimestamp
		}
		if input.SlackTimestamp == thread.ThreadTS {
			parentMessageID = messageID
		}
	}
	contextVersion := thread.ContextVersion
	if changed {
		contextVersion++
	}
	var lastSynced any = nullableTime(thread.LastSyncedAt)
	targetStatus := thread.SyncStatus
	if status == slack.ThreadSynchronized {
		lastSynced = now.UnixMilli()
		targetStatus = slack.ThreadSynchronized
	} else if changed {
		targetStatus = slack.ThreadDirty
	}
	_, err = tx.ExecContext(ctx, `UPDATE slack_threads SET parent_message_id=?, last_message_ts=?, context_version=?,
		sync_status=?, last_synced_at=?, updated_at=? WHERE id=?`, parentMessageID, lastMessageTS, contextVersion,
		targetStatus, lastSynced, now.UnixMilli(), thread.ID)
	if err != nil {
		return slack.Thread{}, false, fmt.Errorf("update Slack thread: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return slack.Thread{}, false, fmt.Errorf("commit Slack message update: %w", err)
	}
	updated, err := store.GetSlackThread(ctx, thread.ID)
	return updated, changed, err
}

type storedAttachmentMetadata struct {
	Filename  string
	MIMEType  string
	SizeBytes int64
	IsRemoved bool
}

func applySlackAttachments(ctx context.Context, tx *sql.Tx, thread slack.Thread, messageID string,
	inputs []slack.AttachmentInput, now time.Time) (bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT slack_file_id, filename, mime_type, size_bytes, is_removed
		FROM slack_attachments WHERE message_id=?`, messageID)
	if err != nil {
		return false, fmt.Errorf("list message attachments: %w", err)
	}
	existing := make(map[string]storedAttachmentMetadata)
	for rows.Next() {
		var fileID string
		var metadata storedAttachmentMetadata
		if err := rows.Scan(&fileID, &metadata.Filename, &metadata.MIMEType, &metadata.SizeBytes, &metadata.IsRemoved); err != nil {
			rows.Close()
			return false, fmt.Errorf("scan message attachment: %w", err)
		}
		existing[fileID] = metadata
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	changed := false
	seen := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		if input.SlackFileID == "" {
			continue
		}
		seen[input.SlackFileID] = true
		filename := input.Filename
		if filename == "" {
			filename = input.SlackFileID
		}
		current, found := existing[input.SlackFileID]
		if !found || current.Filename != filename || current.MIMEType != input.MIMEType ||
			current.SizeBytes != input.SizeBytes || current.IsRemoved {
			changed = true
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO slack_attachments
			(id, workspace_id, message_id, thread_id, slack_file_id, filename, mime_type, size_bytes,
			 download_status, extraction_status, is_removed, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
			ON CONFLICT(workspace_id, slack_file_id) DO UPDATE SET
			 message_id=excluded.message_id, thread_id=excluded.thread_id, filename=excluded.filename,
			 mime_type=excluded.mime_type, size_bytes=excluded.size_bytes, is_removed=0, updated_at=excluded.updated_at`,
			input.LocalID(thread.WorkspaceID), thread.WorkspaceID, messageID, thread.ID, input.SlackFileID,
			filename, input.MIMEType, input.SizeBytes, slack.FilePending, slack.FilePending, now.UnixMilli(), now.UnixMilli())
		if err != nil {
			return false, fmt.Errorf("upsert Slack attachment metadata: %w", err)
		}
	}
	for fileID, current := range existing {
		if !seen[fileID] && !current.IsRemoved {
			changed = true
			if _, err := tx.ExecContext(ctx, `UPDATE slack_attachments SET is_removed=1, updated_at=?
				WHERE workspace_id=? AND slack_file_id=?`, now.UnixMilli(), thread.WorkspaceID, fileID); err != nil {
				return false, fmt.Errorf("mark Slack attachment removed: %w", err)
			}
		}
	}
	return changed, nil
}

func (store *Store) ListSlackMessages(ctx context.Context, threadID string) ([]slack.Message, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT id, thread_id, workspace_id, channel_id, slack_message_ts,
		thread_ts, author_id, text, content_hash, is_parent, is_deleted, edited_at, deleted_at,
		slack_created_at, created_at, updated_at FROM slack_messages WHERE thread_id=?
		ORDER BY slack_created_at, slack_message_ts`, threadID)
	if err != nil {
		return nil, fmt.Errorf("list Slack messages: %w", err)
	}
	defer rows.Close()
	messages := make([]slack.Message, 0)
	for rows.Next() {
		message, err := scanSlackMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, message)
	}
	return messages, rows.Err()
}

func (store *Store) GetSlackMessageByIdentity(ctx context.Context, identity slack.MessageIdentity) (slack.Message, error) {
	row := store.db.QueryRowContext(ctx, `SELECT id, thread_id, workspace_id, channel_id, slack_message_ts,
		thread_ts, author_id, text, content_hash, is_parent, is_deleted, edited_at, deleted_at,
		slack_created_at, created_at, updated_at FROM slack_messages WHERE workspace_id=? AND channel_id=? AND slack_message_ts=?`,
		identity.WorkspaceID, identity.ChannelID, identity.MessageTS)
	return scanSlackMessage(row)
}

func (store *Store) ListSlackAttachments(ctx context.Context, threadID string) ([]slack.Attachment, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT id, message_id, thread_id, slack_file_id, filename, mime_type,
		size_bytes, checksum, local_path, download_status, extraction_status, extractor_name, extractor_version,
		is_removed, created_at, updated_at FROM slack_attachments WHERE thread_id=? ORDER BY created_at, id`, threadID)
	if err != nil {
		return nil, fmt.Errorf("list Slack attachments: %w", err)
	}
	defer rows.Close()
	attachments := make([]slack.Attachment, 0)
	for rows.Next() {
		var attachment slack.Attachment
		var createdAt, updatedAt int64
		if err := rows.Scan(&attachment.ID, &attachment.MessageID, &attachment.ThreadID, &attachment.SlackFileID,
			&attachment.Filename, &attachment.MIMEType, &attachment.SizeBytes, &attachment.Checksum, &attachment.LocalPath,
			&attachment.DownloadStatus, &attachment.ExtractionStatus, &attachment.ExtractorName,
			&attachment.ExtractorVersion, &attachment.IsRemoved, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan Slack attachment: %w", err)
		}
		attachment.CreatedAt = time.UnixMilli(createdAt)
		attachment.UpdatedAt = time.UnixMilli(updatedAt)
		attachments = append(attachments, attachment)
	}
	return attachments, rows.Err()
}

func (store *Store) CreateExtractedDocument(ctx context.Context, document slack.ExtractedDocument) error {
	metadata, err := json.Marshal(document.Metadata)
	if err != nil {
		return fmt.Errorf("encode extracted document metadata: %w", err)
	}
	createdAt := document.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_, err = store.db.ExecContext(ctx, `INSERT INTO extracted_documents
		(id, attachment_id, content, metadata_json, extractor_name, extractor_version, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, document.ID, document.AttachmentID, document.Content, string(metadata),
		document.ExtractorName, document.ExtractorVersion, createdAt.UnixMilli())
	if err != nil {
		return fmt.Errorf("create extracted document: %w", err)
	}
	return nil
}

func (store *Store) ListExtractedDocuments(ctx context.Context, attachmentID string) ([]slack.ExtractedDocument, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT id, attachment_id, content, metadata_json,
		extractor_name, extractor_version, created_at FROM extracted_documents WHERE attachment_id=? ORDER BY created_at`, attachmentID)
	if err != nil {
		return nil, fmt.Errorf("list extracted documents: %w", err)
	}
	defer rows.Close()
	documents := make([]slack.ExtractedDocument, 0)
	for rows.Next() {
		var document slack.ExtractedDocument
		var metadata string
		var createdAt int64
		if err := rows.Scan(&document.ID, &document.AttachmentID, &document.Content, &metadata,
			&document.ExtractorName, &document.ExtractorVersion, &createdAt); err != nil {
			return nil, fmt.Errorf("scan extracted document: %w", err)
		}
		if err := json.Unmarshal([]byte(metadata), &document.Metadata); err != nil {
			return nil, fmt.Errorf("decode extracted document metadata: %w", err)
		}
		document.CreatedAt = time.UnixMilli(createdAt)
		documents = append(documents, document)
	}
	return documents, rows.Err()
}

func (store *Store) LinkSlackWorkflowRun(ctx context.Context, threadID string, contextVersion int64, runID string) error {
	result, err := store.db.ExecContext(ctx, `UPDATE slack_threads SET requested_analysis_version=?, latest_workflow_run_id=?, updated_at=?
		WHERE id=?`, contextVersion, runID, time.Now().UTC().UnixMilli(), threadID)
	if err != nil {
		return fmt.Errorf("link Slack workflow run: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return slack.ErrNotFound
	}
	return nil
}

const slackThreadSelect = `SELECT id, workspace_id, channel_id, thread_ts, parent_message_id,
	last_message_ts, context_version, sync_status, last_synced_at, requested_analysis_version,
	latest_workflow_run_id, created_at, updated_at FROM slack_threads`

type rowScanner interface{ Scan(dest ...any) error }

func scanSlackThread(row rowScanner) (slack.Thread, error) {
	var thread slack.Thread
	var lastSynced sql.NullInt64
	var createdAt, updatedAt int64
	err := row.Scan(&thread.ID, &thread.WorkspaceID, &thread.ChannelID, &thread.ThreadTS, &thread.ParentMessageID,
		&thread.LastMessageTS, &thread.ContextVersion, &thread.SyncStatus, &lastSynced, &thread.RequestedAnalysisVersion,
		&thread.LatestWorkflowRunID, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return slack.Thread{}, slack.ErrNotFound
	}
	if err != nil {
		return slack.Thread{}, fmt.Errorf("scan Slack thread: %w", err)
	}
	thread.LastSyncedAt = fromNullableTime(lastSynced)
	thread.CreatedAt = time.UnixMilli(createdAt)
	thread.UpdatedAt = time.UnixMilli(updatedAt)
	return thread, nil
}

func scanSlackMessage(row rowScanner) (slack.Message, error) {
	var message slack.Message
	var editedAt, deletedAt sql.NullInt64
	var slackCreatedAt, createdAt, updatedAt int64
	err := row.Scan(&message.ID, &message.ThreadID, &message.WorkspaceID, &message.ChannelID,
		&message.SlackTimestamp, &message.ThreadTimestamp, &message.AuthorID, &message.Text, &message.ContentHash,
		&message.IsParent, &message.IsDeleted, &editedAt, &deletedAt, &slackCreatedAt, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return slack.Message{}, slack.ErrNotFound
	}
	if err != nil {
		return slack.Message{}, fmt.Errorf("scan Slack message: %w", err)
	}
	message.EditedAt = fromNullableTime(editedAt)
	message.DeletedAt = fromNullableTime(deletedAt)
	message.SlackCreatedAt = time.UnixMilli(slackCreatedAt)
	message.CreatedAt = time.UnixMilli(createdAt)
	message.UpdatedAt = time.UnixMilli(updatedAt)
	return message, nil
}
