package slack

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidIdentity = errors.New("invalid Slack identity")
	ErrNotFound        = errors.New("Slack record not found")
	ErrEventInProgress = errors.New("Slack event is already being processed")
)

type ThreadSyncStatus string

const (
	ThreadUninitialized ThreadSyncStatus = "uninitialized"
	ThreadSyncing       ThreadSyncStatus = "syncing"
	ThreadSynchronized  ThreadSyncStatus = "synchronized"
	ThreadDirty         ThreadSyncStatus = "dirty"
	ThreadFailed        ThreadSyncStatus = "failed"
)

type ThreadIdentity struct {
	WorkspaceID string
	ChannelID   string
	ThreadTS    string
}

func (identity ThreadIdentity) Validate() error {
	if strings.TrimSpace(identity.WorkspaceID) == "" || strings.TrimSpace(identity.ChannelID) == "" ||
		strings.TrimSpace(identity.ThreadTS) == "" {
		return ErrInvalidIdentity
	}
	return nil
}

func (identity ThreadIdentity) LocalID() string {
	return stableID("thread", identity.WorkspaceID, identity.ChannelID, identity.ThreadTS)
}

type MessageIdentity struct {
	WorkspaceID string
	ChannelID   string
	MessageTS   string
}

func (identity MessageIdentity) LocalID() string {
	return stableID("message", identity.WorkspaceID, identity.ChannelID, identity.MessageTS)
}

func stableID(prefix string, parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + "_" + hex.EncodeToString(hash[:12])
}

type Thread struct {
	ID                       string           `json:"id"`
	WorkspaceID              string           `json:"workspace_id"`
	ChannelID                string           `json:"channel_id"`
	ThreadTS                 string           `json:"thread_ts"`
	ParentMessageID          string           `json:"parent_message_id"`
	LastMessageTS            string           `json:"last_message_ts"`
	ContextVersion           int64            `json:"context_version"`
	SyncStatus               ThreadSyncStatus `json:"sync_status"`
	LastSyncedAt             *time.Time       `json:"last_synced_at,omitempty"`
	RequestedAnalysisVersion int64            `json:"requested_analysis_version"`
	LatestWorkflowRunID      string           `json:"latest_workflow_run_id,omitempty"`
	CreatedAt                time.Time        `json:"created_at"`
	UpdatedAt                time.Time        `json:"updated_at"`
}

func (thread Thread) Identity() ThreadIdentity {
	return ThreadIdentity{WorkspaceID: thread.WorkspaceID, ChannelID: thread.ChannelID, ThreadTS: thread.ThreadTS}
}

type Message struct {
	ID              string     `json:"id"`
	ThreadID        string     `json:"thread_id"`
	WorkspaceID     string     `json:"workspace_id"`
	ChannelID       string     `json:"channel_id"`
	SlackTimestamp  string     `json:"slack_timestamp"`
	ThreadTimestamp string     `json:"thread_timestamp"`
	AuthorID        string     `json:"author_id"`
	Text            string     `json:"text"`
	ContentHash     string     `json:"content_hash"`
	IsParent        bool       `json:"is_parent"`
	IsDeleted       bool       `json:"is_deleted"`
	EditedAt        *time.Time `json:"edited_at,omitempty"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
	SlackCreatedAt  time.Time  `json:"slack_created_at"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type MessageInput struct {
	WorkspaceID      string
	ChannelID        string
	SlackTimestamp   string
	ThreadTimestamp  string
	AuthorID         string
	Text             string
	IsDeleted        bool
	EditedAt         *time.Time
	DeletedAt        *time.Time
	Attachments      []AttachmentInput
	AttachmentsKnown bool
}

type FileStatus string

const (
	FilePending     FileStatus = "pending"
	FileDownloading FileStatus = "downloading"
	FileDownloaded  FileStatus = "downloaded"
	FileExtracting  FileStatus = "extracting"
	FileCompleted   FileStatus = "completed"
	FileUnsupported FileStatus = "unsupported"
	FileFailed      FileStatus = "failed"
)

type AttachmentInput struct {
	SlackFileID string
	Filename    string
	MIMEType    string
	SizeBytes   int64
}

func (attachment AttachmentInput) LocalID(workspaceID string) string {
	return stableID("attachment", workspaceID, attachment.SlackFileID)
}

type Attachment struct {
	ID               string     `json:"id"`
	MessageID        string     `json:"message_id"`
	ThreadID         string     `json:"thread_id"`
	SlackFileID      string     `json:"slack_file_id"`
	Filename         string     `json:"filename"`
	MIMEType         string     `json:"mime_type"`
	SizeBytes        int64      `json:"size_bytes"`
	Checksum         string     `json:"checksum,omitempty"`
	DownloadStatus   FileStatus `json:"download_status"`
	ExtractionStatus FileStatus `json:"extraction_status"`
	ExtractorName    string     `json:"extractor_name,omitempty"`
	ExtractorVersion string     `json:"extractor_version,omitempty"`
	IsRemoved        bool       `json:"is_removed"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	LocalPath        string     `json:"-"`
}

type ExtractedDocument struct {
	ID               string         `json:"id"`
	AttachmentID     string         `json:"attachment_id"`
	Content          string         `json:"content"`
	Metadata         map[string]any `json:"metadata"`
	ExtractorName    string         `json:"extractor_name"`
	ExtractorVersion string         `json:"extractor_version"`
	CreatedAt        time.Time      `json:"created_at"`
}

func (message MessageInput) Identity() MessageIdentity {
	return MessageIdentity{WorkspaceID: message.WorkspaceID, ChannelID: message.ChannelID, MessageTS: message.SlackTimestamp}
}

func (message MessageInput) ContentHash() string {
	hash := sha256.Sum256([]byte(message.Text))
	return hex.EncodeToString(hash[:])
}

func SlackTime(timestamp string) time.Time {
	seconds, fraction, ok := strings.Cut(timestamp, ".")
	if !ok {
		fraction = "0"
	}
	unixSeconds, err := strconv.ParseInt(seconds, 10, 64)
	if err != nil {
		return time.Time{}
	}
	fraction = (fraction + "000000000")[:9]
	nanos, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(unixSeconds, nanos).UTC()
}

type ContextSnapshot struct {
	ThreadID       string
	ContextVersion int64
	Content        string
	Messages       []Message
}

func (snapshot ContextSnapshot) RequestID() string {
	return stableID("request", snapshot.ThreadID, fmt.Sprint(snapshot.ContextVersion))
}
