package cli

import "github.com/trogers1/sidecar-comments/internal/domain"

type commandDefinition struct {
	allowed, required string
	description       string
	mutation          bool
	operation         string
}

var commandDefinitions = map[string]commandDefinition{
	"version":        {description: "Show CLI, format, and protocol versions."},
	"validate":       {description: "Validate comments and report repair needs.", allowed: "file source"},
	"format":         {description: "Format the sidecar canonically.", allowed: "file check", mutation: true},
	"fix":            {description: "Repair missing message IDs.", allowed: "file check add-missing-message-ids", mutation: true},
	"thread-list":    {description: "List threads, locations, and messages.", allowed: "file"},
	"thread-get":     {description: "Show one thread and its messages.", allowed: "file id", required: "id"},
	"thread-create":  {description: "Create a thread with its first message.", allowed: "file location author-kind author-name body-file external-author-id id message-id gitlab-json", required: "location author-kind body-file", mutation: true, operation: domain.OpThreadCreate},
	"thread-move":    {description: "Move a thread to a new location.", allowed: "file id location", required: "id location", mutation: true, operation: domain.OpThreadMove},
	"thread-delete":  {description: "Delete a thread and all its messages.", allowed: "file id", required: "id", mutation: true, operation: domain.OpThreadDelete},
	"message-add":    {description: "Append a message to a thread.", allowed: "file thread author-kind author-name body-file external-author-id id gitlab-json if-last-author-kind", required: "thread author-kind body-file", mutation: true, operation: domain.OpMessageAdd},
	"message-update": {description: "Replace a message body.", allowed: "file thread id body-file", required: "thread id body-file", mutation: true, operation: domain.OpMessageUpdate},
	"message-delete": {description: "Delete a message from a thread.", allowed: "file thread id", required: "thread id", mutation: true, operation: domain.OpMessageDelete},
	"apply":          {description: "Apply a batch of operations atomically.", allowed: "file operations-file", required: "operations-file", mutation: true},
}
