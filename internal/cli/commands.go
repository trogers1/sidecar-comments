package cli

import "github.com/trogers1/sidecar-comments/internal/domain"

type commandDefinition struct {
	allowed, required string
	mutation          bool
	operation         string
}

var commandDefinitions = map[string]commandDefinition{
	"version":        {},
	"validate":       {allowed: "file source"},
	"format":         {allowed: "file check", mutation: true},
	"fix":            {allowed: "file check add-missing-message-ids", mutation: true},
	"thread-list":    {allowed: "file"},
	"thread-get":     {allowed: "file id", required: "id"},
	"thread-create":  {allowed: "file location author-kind author-name body-file external-author-id id message-id gitlab-json", required: "location author-kind body-file", mutation: true, operation: domain.OpThreadCreate},
	"thread-move":    {allowed: "file id location", required: "id location", mutation: true, operation: domain.OpThreadMove},
	"thread-delete":  {allowed: "file id", required: "id", mutation: true, operation: domain.OpThreadDelete},
	"message-add":    {allowed: "file thread author-kind author-name body-file external-author-id id gitlab-json if-last-author-kind", required: "thread author-kind body-file", mutation: true, operation: domain.OpMessageAdd},
	"message-update": {allowed: "file thread id body-file", required: "thread id body-file", mutation: true, operation: domain.OpMessageUpdate},
	"message-delete": {allowed: "file thread id", required: "thread id", mutation: true, operation: domain.OpMessageDelete},
	"apply":          {allowed: "file operations-file", required: "operations-file", mutation: true},
}
