package slack

import (
	"strings"
	"testing"
	"time"
)

func TestBuildContextKeepsParentAndLatestActiveReplies(t *testing.T) {
	base := time.Unix(1710000000, 0)
	messages := []Message{
		{ID: "parent", SlackTimestamp: "1710000000.000001", AuthorID: "U1", Text: "Parent", IsParent: true, SlackCreatedAt: base},
		{ID: "old", SlackTimestamp: "1710000001.000001", AuthorID: "U2", Text: "Old", SlackCreatedAt: base.Add(time.Second)},
		{ID: "deleted", SlackTimestamp: "1710000002.000001", AuthorID: "U3", Text: "Deleted", IsDeleted: true, SlackCreatedAt: base.Add(2 * time.Second)},
		{ID: "latest", SlackTimestamp: "1710000003.000001", AuthorID: "U4", Text: "Latest", SlackCreatedAt: base.Add(3 * time.Second)},
	}
	snapshot, err := BuildContext(Thread{ID: "thread_1", ContextVersion: 3}, messages, 2, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Messages) != 2 || snapshot.Messages[0].ID != "parent" || snapshot.Messages[1].ID != "latest" {
		t.Fatalf("selected messages = %+v", snapshot.Messages)
	}
	if strings.Contains(snapshot.Content, "Deleted") || strings.Contains(snapshot.Content, "Old") {
		t.Fatalf("bounded context = %q", snapshot.Content)
	}
	if len([]rune(snapshot.Content)) > 200 {
		t.Fatalf("context length = %d", len([]rune(snapshot.Content)))
	}
}
