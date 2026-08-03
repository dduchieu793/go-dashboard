package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dduchieu793/go-dashboard/backend/internal/trigger"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
)

var (
	ErrUnsupportedEvent = errors.New("unsupported Slack event")
	ErrEmptyContext     = errors.New("Slack thread has no active text context")
)

type EventEnvelope struct {
	Type      string          `json:"type"`
	Challenge string          `json:"challenge"`
	TeamID    string          `json:"team_id"`
	EventID   string          `json:"event_id"`
	EventTime int64           `json:"event_time"`
	Event     json.RawMessage `json:"event"`
}

type messageEvent struct {
	Type            string        `json:"type"`
	Subtype         string        `json:"subtype"`
	Channel         string        `json:"channel"`
	User            string        `json:"user"`
	Text            string        `json:"text"`
	TS              string        `json:"ts"`
	ThreadTS        string        `json:"thread_ts"`
	DeletedTS       string        `json:"deleted_ts"`
	Message         *messageEvent `json:"message"`
	PreviousMessage *messageEvent `json:"previous_message"`
	Edited          *struct {
		TS string `json:"ts"`
	} `json:"edited"`
	Files []slackFile `json:"files"`
}

type Repository interface {
	ClaimSlackEvent(context.Context, string, time.Time) (bool, error)
	CompleteSlackEvent(context.Context, string, time.Time) error
	ReleaseSlackEvent(context.Context, string) error
	EnsureSlackThread(context.Context, ThreadIdentity, ThreadSyncStatus) (Thread, bool, error)
	GetSlackThreadByIdentity(context.Context, ThreadIdentity) (Thread, error)
	GetSlackThread(context.Context, string) (Thread, error)
	ListSlackThreads(context.Context, int) ([]Thread, error)
	UpdateSlackThreadStatus(context.Context, string, ThreadSyncStatus) error
	ApplySlackMessages(context.Context, string, []MessageInput, ThreadSyncStatus) (Thread, bool, error)
	ListSlackMessages(context.Context, string) ([]Message, error)
	GetSlackMessageByIdentity(context.Context, MessageIdentity) (Message, error)
	ListSlackAttachments(context.Context, string) ([]Attachment, error)
	LinkSlackWorkflowRun(context.Context, string, int64, string) error
}

type WorkflowStarter interface {
	Start(context.Context, trigger.Request) (workflow.Run, error)
}

type Service struct {
	repository  Repository
	client      ThreadClient
	workflows   WorkflowStarter
	maxMessages int
	maxChars    int
}

func NewService(repository Repository, client ThreadClient, workflows WorkflowStarter, maxMessages, maxChars int) *Service {
	return &Service{repository: repository, client: client, workflows: workflows, maxMessages: maxMessages, maxChars: maxChars}
}

func (service *Service) HandleEvent(ctx context.Context, envelope EventEnvelope) (workflow.Run, bool, error) {
	if envelope.Type != "event_callback" || envelope.EventID == "" || envelope.TeamID == "" {
		return workflow.Run{}, false, ErrUnsupportedEvent
	}
	receivedAt := time.Now().UTC()
	if envelope.EventTime > 0 {
		receivedAt = time.Unix(envelope.EventTime, 0).UTC()
	}
	claimed, err := service.repository.ClaimSlackEvent(ctx, envelope.EventID, receivedAt)
	if err != nil || !claimed {
		return workflow.Run{}, false, err
	}
	completed := false
	defer func() {
		if !completed {
			_ = service.repository.ReleaseSlackEvent(context.Background(), envelope.EventID)
		}
	}()
	input, err := normalizeEvent(envelope)
	if err != nil {
		if errors.Is(err, ErrUnsupportedEvent) {
			if completeErr := service.repository.CompleteSlackEvent(ctx, envelope.EventID, time.Now().UTC()); completeErr != nil {
				return workflow.Run{}, false, completeErr
			}
			completed = true
			return workflow.Run{}, false, nil
		}
		return workflow.Run{}, false, err
	}
	if input.ThreadTimestamp == "" && input.IsDeleted {
		existing, getErr := service.repository.GetSlackMessageByIdentity(ctx, input.Identity())
		if getErr != nil {
			return workflow.Run{}, false, getErr
		}
		input.ThreadTimestamp = existing.ThreadTimestamp
	}
	identity := ThreadIdentity{WorkspaceID: input.WorkspaceID, ChannelID: input.ChannelID, ThreadTS: input.ThreadTimestamp}
	thread, err := service.repository.GetSlackThreadByIdentity(ctx, identity)
	needsSync := errors.Is(err, ErrNotFound)
	if err != nil && !needsSync {
		return workflow.Run{}, false, err
	}
	if needsSync {
		thread, _, err = service.repository.EnsureSlackThread(ctx, identity, ThreadSyncing)
		if err != nil {
			return workflow.Run{}, false, err
		}
	}
	changed := false
	if needsSync || thread.SyncStatus == ThreadUninitialized || thread.SyncStatus == ThreadSyncing || thread.SyncStatus == ThreadFailed {
		thread, changed, err = service.synchronize(ctx, thread)
		if err != nil {
			return workflow.Run{}, false, err
		}
	}
	thread, incrementalChanged, err := service.repository.ApplySlackMessages(ctx, thread.ID, []MessageInput{input}, ThreadDirty)
	if err != nil {
		return workflow.Run{}, false, err
	}
	changed = changed || incrementalChanged
	var run workflow.Run
	if changed && thread.ContextVersion > thread.RequestedAnalysisVersion {
		run, err = service.Analyze(ctx, thread.ID)
		if err != nil {
			return workflow.Run{}, false, err
		}
	}
	if err := service.repository.CompleteSlackEvent(ctx, envelope.EventID, time.Now().UTC()); err != nil {
		return workflow.Run{}, false, err
	}
	completed = true
	return run, run.ID != "", nil
}

func (service *Service) synchronize(ctx context.Context, thread Thread) (Thread, bool, error) {
	if err := service.repository.UpdateSlackThreadStatus(ctx, thread.ID, ThreadSyncing); err != nil {
		return Thread{}, false, err
	}
	messages, err := service.client.FetchThread(ctx, thread.ChannelID, thread.ThreadTS)
	if err != nil {
		_ = service.repository.UpdateSlackThreadStatus(ctx, thread.ID, ThreadFailed)
		return Thread{}, false, err
	}
	for index := range messages {
		messages[index].WorkspaceID = thread.WorkspaceID
		messages[index].ChannelID = thread.ChannelID
		messages[index].ThreadTimestamp = thread.ThreadTS
	}
	return service.repository.ApplySlackMessages(ctx, thread.ID, messages, ThreadSynchronized)
}

func (service *Service) Refresh(ctx context.Context, threadID string) (Thread, workflow.Run, error) {
	thread, err := service.repository.GetSlackThread(ctx, threadID)
	if err != nil {
		return Thread{}, workflow.Run{}, err
	}
	thread, changed, err := service.synchronize(ctx, thread)
	if err != nil {
		return Thread{}, workflow.Run{}, err
	}
	if changed && thread.ContextVersion > thread.RequestedAnalysisVersion {
		run, analyzeErr := service.Analyze(ctx, thread.ID)
		return thread, run, analyzeErr
	}
	return thread, workflow.Run{}, nil
}

func (service *Service) Analyze(ctx context.Context, threadID string) (workflow.Run, error) {
	thread, err := service.repository.GetSlackThread(ctx, threadID)
	if err != nil {
		return workflow.Run{}, err
	}
	messages, err := service.repository.ListSlackMessages(ctx, thread.ID)
	if err != nil {
		return workflow.Run{}, err
	}
	snapshot, err := BuildContext(thread, messages, service.maxMessages, service.maxChars)
	if err != nil {
		return workflow.Run{}, err
	}
	sources := make([]trigger.Source, 0, len(snapshot.Messages))
	for _, message := range snapshot.Messages {
		sources = append(sources, trigger.Source{ID: message.ID, Kind: "slack_message", ExternalID: message.SlackTimestamp,
			AuthorID: message.AuthorID, Content: message.Text, OccurredAt: message.SlackCreatedAt,
			Metadata: map[string]string{"channel_id": message.ChannelID, "thread_ts": message.ThreadTimestamp}})
	}
	run, err := service.workflows.Start(ctx, trigger.Request{
		ID: snapshot.RequestID(), Source: "slack", Type: "slack_thread", Content: snapshot.Content,
		Metadata: map[string]string{"slack_thread_id": thread.ID, "workspace_id": thread.WorkspaceID,
			"channel_id": thread.ChannelID, "thread_ts": thread.ThreadTS, "context_version": strconv.FormatInt(thread.ContextVersion, 10)},
		Sources: sources, ReceivedAt: time.Now().UTC(),
	})
	if err != nil {
		return workflow.Run{}, err
	}
	if err := service.repository.LinkSlackWorkflowRun(ctx, thread.ID, thread.ContextVersion, run.ID); err != nil {
		return workflow.Run{}, err
	}
	return run, nil
}

func (service *Service) ListThreads(ctx context.Context, limit int) ([]Thread, error) {
	return service.repository.ListSlackThreads(ctx, limit)
}

func (service *Service) GetThread(ctx context.Context, id string) (Thread, error) {
	return service.repository.GetSlackThread(ctx, id)
}

func (service *Service) ListMessages(ctx context.Context, id string) ([]Message, error) {
	if _, err := service.repository.GetSlackThread(ctx, id); err != nil {
		return nil, err
	}
	return service.repository.ListSlackMessages(ctx, id)
}

func (service *Service) ListAttachments(ctx context.Context, id string) ([]Attachment, error) {
	if _, err := service.repository.GetSlackThread(ctx, id); err != nil {
		return nil, err
	}
	return service.repository.ListSlackAttachments(ctx, id)
}

func normalizeEvent(envelope EventEnvelope) (MessageInput, error) {
	var event messageEvent
	if err := json.Unmarshal(envelope.Event, &event); err != nil {
		return MessageInput{}, fmt.Errorf("decode Slack message event: %w", err)
	}
	if event.Type != "message" {
		return MessageInput{}, ErrUnsupportedEvent
	}
	channel := event.Channel
	message := &event
	deleted := false
	var deletedAt *time.Time
	switch event.Subtype {
	case "", "thread_broadcast", "bot_message":
	case "message_changed":
		if event.Message == nil {
			return MessageInput{}, ErrUnsupportedEvent
		}
		message = event.Message
	case "message_deleted":
		message = event.PreviousMessage
		if message == nil {
			message = &messageEvent{TS: event.DeletedTS}
		}
		if message.TS == "" {
			message.TS = event.DeletedTS
		}
		deleted = true
		now := time.Now().UTC()
		deletedAt = &now
	default:
		return MessageInput{}, ErrUnsupportedEvent
	}
	if channel == "" {
		channel = message.Channel
	}
	threadTS := message.ThreadTS
	if threadTS == "" && !deleted {
		threadTS = message.TS
	}
	var editedAt *time.Time
	if message.Edited != nil {
		parsed := SlackTime(message.Edited.TS)
		if !parsed.IsZero() {
			editedAt = &parsed
		}
	}
	if channel == "" || message.TS == "" {
		return MessageInput{}, ErrUnsupportedEvent
	}
	attachments := attachmentInputs(message.Files)
	if deleted {
		attachments = []AttachmentInput{}
	}
	return MessageInput{WorkspaceID: envelope.TeamID, ChannelID: channel, SlackTimestamp: message.TS,
		ThreadTimestamp: threadTS, AuthorID: message.User, Text: message.Text, IsDeleted: deleted,
		EditedAt: editedAt, DeletedAt: deletedAt, Attachments: attachments, AttachmentsKnown: true}, nil
}

func BuildContext(thread Thread, messages []Message, maxMessages, maxChars int) (ContextSnapshot, error) {
	if maxMessages < 1 {
		maxMessages = 200
	}
	if maxChars < 1 {
		maxChars = 50000
	}
	active := make([]Message, 0, len(messages))
	for _, message := range messages {
		if !message.IsDeleted && strings.TrimSpace(message.Text) != "" {
			active = append(active, message)
		}
	}
	if len(active) == 0 {
		return ContextSnapshot{}, ErrEmptyContext
	}
	sort.SliceStable(active, func(i, j int) bool {
		if active[i].SlackCreatedAt.Equal(active[j].SlackCreatedAt) {
			return active[i].SlackTimestamp < active[j].SlackTimestamp
		}
		return active[i].SlackCreatedAt.Before(active[j].SlackCreatedAt)
	})
	var parent *Message
	replies := make([]Message, 0, len(active))
	for index := range active {
		if active[index].IsParent && parent == nil {
			copy := active[index]
			parent = &copy
		} else {
			replies = append(replies, active[index])
		}
	}
	selected := make([]Message, 0, maxMessages)
	budget := maxChars
	if parent != nil {
		copy := *parent
		parentBudget := budget
		if len(replies) > 0 && maxMessages > 1 {
			parentBudget = budget / 2
		}
		copy.Text = truncateText(copy.Text, textBudget(copy, parentBudget))
		if copy.Text != "" {
			budget -= renderedLength(copy)
			selected = append(selected, copy)
		}
	}
	remainingSlots := maxMessages - len(selected)
	for index := len(replies) - 1; index >= 0 && remainingSlots > 0 && budget > 0; index-- {
		copy := replies[index]
		copy.Text = truncateText(copy.Text, textBudget(copy, budget))
		if copy.Text == "" {
			continue
		}
		budget -= renderedLength(copy)
		selected = append(selected, copy)
		remainingSlots--
	}
	sort.SliceStable(selected, func(i, j int) bool { return selected[i].SlackTimestamp < selected[j].SlackTimestamp })
	var content strings.Builder
	for _, message := range selected {
		fmt.Fprintf(&content, "%s%s\n\n", messagePrefix(message), message.Text)
	}
	return ContextSnapshot{ThreadID: thread.ID, ContextVersion: thread.ContextVersion,
		Content: strings.TrimSpace(content.String()), Messages: selected}, nil
}

func messagePrefix(message Message) string {
	role := "reply"
	if message.IsParent {
		role = "parent"
	}
	return fmt.Sprintf("[Slack %s ts=%s author=%s]\n", role, message.SlackTimestamp, message.AuthorID)
}

func renderedLength(message Message) int {
	return len([]rune(messagePrefix(message))) + len([]rune(message.Text)) + 2
}

func textBudget(message Message, totalBudget int) int {
	return totalBudget - len([]rune(messagePrefix(message))) - 2
}

func truncateText(value string, max int) string {
	runes := []rune(strings.TrimSpace(value))
	if max <= 0 {
		return ""
	}
	if len(runes) <= max {
		return string(runes)
	}
	if max <= 1 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}
