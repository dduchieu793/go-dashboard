package slack

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestStableSlackIdentities(t *testing.T) {
	thread := ThreadIdentity{WorkspaceID: "T1", ChannelID: "C1", ThreadTS: "1710000000.000001"}
	if err := thread.Validate(); err != nil {
		t.Fatal(err)
	}
	if thread.LocalID() != (ThreadIdentity{WorkspaceID: "T1", ChannelID: "C1", ThreadTS: "1710000000.000001"}).LocalID() {
		t.Fatal("thread IDs must be deterministic")
	}
	first := MessageIdentity{WorkspaceID: "T1", ChannelID: "C1", MessageTS: thread.ThreadTS}.LocalID()
	second := MessageIdentity{WorkspaceID: "T1", ChannelID: "C1", MessageTS: "1710000001.000001"}.LocalID()
	if first == second {
		t.Fatal("different Slack messages must have different IDs")
	}
	if err := (ThreadIdentity{}).Validate(); !errors.Is(err, ErrInvalidIdentity) {
		t.Fatalf("empty identity error = %v", err)
	}
}

func TestAttachmentJSONDoesNotExposeLocalPath(t *testing.T) {
	encoded, err := json.Marshal(Attachment{ID: "attachment_1", Filename: "plan.pdf", LocalPath: "/private/file"})
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" || strings.Contains(string(encoded), "private") || strings.Contains(string(encoded), "local_path") {
		t.Fatalf("attachment JSON = %s", encoded)
	}
}

func TestSlackTime(t *testing.T) {
	parsed := SlackTime("1710000000.123456")
	if parsed.Unix() != 1710000000 || parsed.Nanosecond() != 123456000 {
		t.Fatalf("SlackTime() = %s", parsed)
	}
}
