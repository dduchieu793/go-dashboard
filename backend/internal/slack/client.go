package slack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrAPI = errors.New("Slack API request failed")

type ThreadClient interface {
	FetchThread(ctx context.Context, channelID, threadTS string) ([]MessageInput, error)
}

type APIClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewAPIClient(baseURL, token string, timeout time.Duration) *APIClient {
	return &APIClient{baseURL: strings.TrimRight(baseURL, "/"), token: token, client: &http.Client{Timeout: timeout}}
}

type slackMessage struct {
	Type     string `json:"type"`
	Subtype  string `json:"subtype"`
	User     string `json:"user"`
	Text     string `json:"text"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
	Edited   *struct {
		TS string `json:"ts"`
	} `json:"edited"`
	Files []slackFile `json:"files"`
}

type slackFile struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	MIMEType string `json:"mimetype"`
	Size     int64  `json:"size"`
}

type repliesResponse struct {
	OK       bool           `json:"ok"`
	Error    string         `json:"error"`
	Messages []slackMessage `json:"messages"`
	Metadata struct {
		NextCursor string `json:"next_cursor"`
	} `json:"response_metadata"`
}

func (client *APIClient) FetchThread(ctx context.Context, channelID, threadTS string) ([]MessageInput, error) {
	cursor := ""
	messages := make([]MessageInput, 0)
	for {
		query := url.Values{"channel": {channelID}, "ts": {threadTS}, "limit": {"200"}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var page repliesResponse
		if err := client.get(ctx, "/conversations.replies?"+query.Encode(), &page); err != nil {
			return nil, err
		}
		if !page.OK {
			return nil, fmt.Errorf("%w: conversations.replies: %s", ErrAPI, page.Error)
		}
		for _, item := range page.Messages {
			if item.TS == "" {
				continue
			}
			resolvedThreadTS := item.ThreadTS
			if resolvedThreadTS == "" {
				resolvedThreadTS = threadTS
			}
			var editedAt *time.Time
			if item.Edited != nil {
				parsed := SlackTime(item.Edited.TS)
				if !parsed.IsZero() {
					editedAt = &parsed
				}
			}
			messages = append(messages, MessageInput{ChannelID: channelID, SlackTimestamp: item.TS,
				ThreadTimestamp: resolvedThreadTS, AuthorID: item.User, Text: item.Text, EditedAt: editedAt,
				Attachments: attachmentInputs(item.Files), AttachmentsKnown: true})
		}
		cursor = strings.TrimSpace(page.Metadata.NextCursor)
		if cursor == "" {
			return messages, nil
		}
	}
}

func attachmentInputs(files []slackFile) []AttachmentInput {
	attachments := make([]AttachmentInput, 0, len(files))
	for _, file := range files {
		if file.ID == "" {
			continue
		}
		attachments = append(attachments, AttachmentInput{SlackFileID: file.ID, Filename: file.Name,
			MIMEType: file.MIMEType, SizeBytes: file.Size})
	}
	return attachments
}

func (client *APIClient) get(ctx context.Context, path string, target any) error {
	for attempt := 0; attempt < 3; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+path, nil)
		if err != nil {
			return err
		}
		request.Header.Set("Authorization", "Bearer "+client.token)
		response, err := client.client.Do(request)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrAPI, err)
		}
		if response.StatusCode == http.StatusTooManyRequests {
			_ = response.Body.Close()
			delay := time.Second
			if seconds, parseErr := strconv.Atoi(response.Header.Get("Retry-After")); parseErr == nil && seconds > 0 {
				delay = time.Duration(seconds) * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
				continue
			}
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			_ = response.Body.Close()
			return fmt.Errorf("%w: HTTP %d", ErrAPI, response.StatusCode)
		}
		err = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(target)
		_ = response.Body.Close()
		if err != nil {
			return fmt.Errorf("decode Slack API response: %w", err)
		}
		return nil
	}
	return fmt.Errorf("%w: rate limit retry exhausted", ErrAPI)
}
