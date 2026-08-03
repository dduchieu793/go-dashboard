package slack_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/dduchieu793/go-dashboard/backend/internal/slack"
	"github.com/dduchieu793/go-dashboard/backend/internal/storage"
	"github.com/dduchieu793/go-dashboard/backend/internal/trigger"
	"github.com/dduchieu793/go-dashboard/backend/internal/workflow"
)

type threadClientStub struct {
	calls    int
	messages []slack.MessageInput
}

func (client *threadClientStub) FetchThread(context.Context, string, string) ([]slack.MessageInput, error) {
	client.calls++
	return append([]slack.MessageInput{}, client.messages...), nil
}

type workflowStarterStub struct{ requests []trigger.Request }

func (starter *workflowStarterStub) Start(_ context.Context, request trigger.Request) (workflow.Run, error) {
	starter.requests = append(starter.requests, request)
	return workflow.Run{ID: "run_" + request.ID, Request: request}, nil
}

func eventEnvelope(t *testing.T, eventID, event string) slack.EventEnvelope {
	t.Helper()
	return slack.EventEnvelope{Type: "event_callback", TeamID: "T1", EventID: eventID, Event: json.RawMessage(event)}
}

func TestSlackServiceSynchronizesThenProcessesIncrementally(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "dashboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	client := &threadClientStub{messages: []slack.MessageInput{
		{SlackTimestamp: "1710000000.000001", AuthorID: "U1", Text: "Parent", AttachmentsKnown: true,
			Attachments: []slack.AttachmentInput{{SlackFileID: "F1", Filename: "plan.pdf", MIMEType: "application/pdf", SizeBytes: 1234}}},
		{SlackTimestamp: "1710000001.000001", AuthorID: "U2", Text: "First reply", AttachmentsKnown: true},
	}}
	workflows := &workflowStarterStub{}
	service := slack.NewService(store, client, workflows, 100, 10000)
	ctx := context.Background()

	first := eventEnvelope(t, "Ev1", `{"type":"message","channel":"C1","ts":"1710000001.000001","thread_ts":"1710000000.000001","user":"U2","text":"First reply"}`)
	run, started, err := service.HandleEvent(ctx, first)
	if err != nil || !started || run.ID == "" {
		t.Fatalf("first event = run %+v, started %t, error %v", run, started, err)
	}
	threads, err := service.ListThreads(ctx, 10)
	if err != nil || len(threads) != 1 || threads[0].ContextVersion != 1 {
		t.Fatalf("threads = %+v, error %v", threads, err)
	}
	if client.calls != 1 || len(workflows.requests) != 1 || len(workflows.requests[0].Sources) != 2 {
		t.Fatalf("client calls=%d workflow requests=%+v", client.calls, workflows.requests)
	}
	attachments, err := service.ListAttachments(ctx, threads[0].ID)
	if err != nil || len(attachments) != 1 || attachments[0].Filename != "plan.pdf" ||
		attachments[0].DownloadStatus != slack.FilePending || attachments[0].LocalPath != "" {
		t.Fatalf("attachments = %+v, error %v", attachments, err)
	}
	if err := store.CreateExtractedDocument(ctx, slack.ExtractedDocument{ID: "document_1", AttachmentID: attachments[0].ID,
		Content: "future extracted content", Metadata: map[string]any{"pages": float64(1)},
		ExtractorName: "test", ExtractorVersion: "v1"}); err != nil {
		t.Fatal(err)
	}
	documents, err := store.ListExtractedDocuments(ctx, attachments[0].ID)
	if err != nil || len(documents) != 1 || documents[0].Metadata["pages"] != float64(1) {
		t.Fatalf("documents = %+v, error %v", documents, err)
	}
	if workflows.requests[0].Metadata["context_version"] != "1" || workflows.requests[0].Type != "slack_thread" {
		t.Fatalf("normalized request = %+v", workflows.requests[0])
	}

	if _, duplicateStarted, err := service.HandleEvent(ctx, first); err != nil || duplicateStarted {
		t.Fatalf("duplicate event started=%t error=%v", duplicateStarted, err)
	}
	second := eventEnvelope(t, "Ev2", `{"type":"message","channel":"C1","ts":"1710000002.000001","thread_ts":"1710000000.000001","user":"U3","text":"Second reply","files":[{"id":"F2","name":"orders.csv","mimetype":"text/csv","size":42}]}`)
	if _, started, err := service.HandleEvent(ctx, second); err != nil || !started {
		t.Fatalf("incremental event started=%t error=%v", started, err)
	}
	threads, _ = service.ListThreads(ctx, 10)
	if client.calls != 1 || threads[0].ContextVersion != 2 || threads[0].SyncStatus != slack.ThreadDirty || len(workflows.requests) != 2 {
		t.Fatalf("after incremental: client=%d thread=%+v workflows=%d", client.calls, threads[0], len(workflows.requests))
	}
	attachments, _ = service.ListAttachments(ctx, threads[0].ID)
	if len(attachments) != 2 || attachments[1].SlackFileID != "F2" {
		t.Fatalf("incremental attachments = %+v", attachments)
	}

	same := eventEnvelope(t, "Ev3", `{"type":"message","channel":"C1","ts":"1710000002.000001","thread_ts":"1710000000.000001","user":"U3","text":"Second reply","files":[{"id":"F2","name":"orders.csv","mimetype":"text/csv","size":42}]}`)
	if _, started, err := service.HandleEvent(ctx, same); err != nil || started {
		t.Fatalf("unchanged event started=%t error=%v", started, err)
	}

	edited := eventEnvelope(t, "Ev4", `{"type":"message","subtype":"message_changed","channel":"C1","message":{"type":"message","ts":"1710000002.000001","thread_ts":"1710000000.000001","user":"U3","text":"Edited reply","edited":{"ts":"1710000003.000001"}}}`)
	if _, started, err := service.HandleEvent(ctx, edited); err != nil || !started {
		t.Fatalf("edited event started=%t error=%v", started, err)
	}
	attachments, _ = service.ListAttachments(ctx, threads[0].ID)
	if len(attachments) != 2 || !attachments[1].IsRemoved {
		t.Fatalf("edited attachment state = %+v", attachments)
	}
	deleted := eventEnvelope(t, "Ev5", `{"type":"message","subtype":"message_deleted","channel":"C1","deleted_ts":"1710000002.000001","previous_message":{"ts":"1710000002.000001","thread_ts":"1710000000.000001","user":"U3","text":"Edited reply"}}`)
	if _, started, err := service.HandleEvent(ctx, deleted); err != nil || !started {
		t.Fatalf("deleted event started=%t error=%v", started, err)
	}
	threads, _ = service.ListThreads(ctx, 10)
	if threads[0].ContextVersion != 4 || len(workflows.requests) != 4 {
		t.Fatalf("final thread=%+v workflows=%d", threads[0], len(workflows.requests))
	}
	last := workflows.requests[len(workflows.requests)-1]
	if len(last.Sources) != 2 {
		t.Fatalf("deleted message remained in sources: %+v", last.Sources)
	}
}
