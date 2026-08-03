package slack

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAPIClientFetchThreadPaginates(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		requests++
		if request.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("Authorization = %q", request.Header.Get("Authorization"))
		}
		if requests == 1 {
			fmt.Fprint(response, `{"ok":true,"messages":[{"ts":"1710000000.000001","user":"U1","text":"parent","files":[{"id":"F1","name":"plan.pdf","mimetype":"application/pdf","size":1234}]}],"response_metadata":{"next_cursor":"next"}}`)
			return
		}
		if request.URL.Query().Get("cursor") != "next" {
			t.Fatalf("cursor = %q", request.URL.Query().Get("cursor"))
		}
		fmt.Fprint(response, `{"ok":true,"messages":[{"ts":"1710000001.000001","thread_ts":"1710000000.000001","user":"U2","text":"reply"}],"response_metadata":{"next_cursor":""}}`)
	}))
	defer server.Close()
	client := NewAPIClient(server.URL, "token", time.Second)
	messages, err := client.FetchThread(context.Background(), "C1", "1710000000.000001")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(messages) != 2 || messages[0].ThreadTimestamp != "1710000000.000001" {
		t.Fatalf("requests=%d messages=%+v", requests, messages)
	}
	if !messages[0].AttachmentsKnown || len(messages[0].Attachments) != 1 || messages[0].Attachments[0].SlackFileID != "F1" {
		t.Fatalf("attachments = %+v", messages[0].Attachments)
	}
}
