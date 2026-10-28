package domain

import "encoding/json"

const (
	MaxTargetBytes      = 4194304
	MaxSourceBytes      = 16777216
	MaxThreads          = 1024
	MaxMessagesThread   = 256
	MaxMessagesDocument = 16384
	MaxAnchorBytes      = 4096
	MaxBodyBytes        = 65536
	MaxMetadataBytes    = 16384
	MaxApplyBytes       = 1048576
	MaxApplyOperations  = 256
)

// Location is an untrusted wire shape, not a validated immutable value.
type Location struct {
	Kind        string  `json:"kind"`
	Line        int     `json:"line,omitempty"`
	StartLine   int     `json:"start_line,omitempty"`
	StartColumn int     `json:"start_column,omitempty"`
	EndLine     int     `json:"end_line,omitempty"`
	EndColumn   int     `json:"end_column,omitempty"`
	Anchor      *string `json:"anchor,omitempty"`
}

// ExternalAuthor is an untrusted wire shape.
type ExternalAuthor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Author is an untrusted wire shape; validate its closed union before use.
type Author struct {
	Kind           string          `json:"kind"`
	Name           *string         `json:"name,omitempty"`
	ExternalAuthor *ExternalAuthor `json:"external_author,omitempty"`
}

// GitLab is an untrusted wire shape; validate its closed union before use.
type GitLab struct {
	Kind         string  `json:"kind"`
	ProjectID    int64   `json:"project_id"`
	DiscussionID *string `json:"discussion_id,omitempty"`
	NoteID       int64   `json:"note_id"`
}

// Message is an untrusted wire shape. DocumentValue owns validated messages.
type Message struct {
	ID     string  `json:"id"`
	Author Author  `json:"author"`
	Body   string  `json:"body"`
	GitLab *GitLab `json:"gitlab,omitempty"`
}

// Thread is an untrusted wire shape. Construct DocumentValue at the domain boundary.
type Thread struct {
	ID       string    `json:"id"`
	Location Location  `json:"location"`
	Messages []Message `json:"messages"`
}

// Diagnostic is the repair-report wire shape, not a validated document value.
type Diagnostic struct {
	Code           string `json:"code"`
	ByteOffset     int    `json:"byte_offset"`
	Line           int    `json:"line"`
	ThreadID       string `json:"thread_id"`
	MessageOrdinal int    `json:"message_ordinal"`
}
type Error struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details"`
	Exit    int             `json:"-"`
}

func (e *Error) Error() string { return e.Message }

type ValidationRequest struct{ Path, Reason string }

func Validation(r ValidationRequest) error {
	b, err := json.Marshal(ValidationDetails{Path: r.Path, Reason: r.Reason})
	if err != nil {
		panic(err)
	}
	return &Error{Code: CodeValidation, Message: CodeValidation + ":" + r.Path + ":" + r.Reason, Details: b, Exit: ExitValidation}
}

type IORequest struct {
	Operation, Path        string
	CommitMayHaveSucceeded *bool
}

func IO(r IORequest) error {
	b, err := json.Marshal(IODetails{Operation: r.Operation, Path: r.Path, CommitMayHaveSucceeded: r.CommitMayHaveSucceeded})
	if err != nil {
		panic(err)
	}
	return &Error{Code: CodeIO, Message: CodeIO + ":" + r.Operation, Details: b, Exit: ExitIO}
}
