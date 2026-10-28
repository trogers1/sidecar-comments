package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
)

const (
	ProtocolVersion    = 1
	FormatVersion      = 1
	ExitUsage          = 2
	ExitValidation     = 3
	ExitConflict       = 4
	ExitNotFound       = 5
	ExitPrecondition   = 6
	ExitIO             = 7
	ExitUnsupported    = 8
	CodeUsage          = "usage"
	CodeValidation     = "validation"
	CodeConflict       = "conflict"
	CodeNotFound       = "not_found"
	CodePrecondition   = "precondition"
	CodeIO             = "io"
	CodeUnsupported    = "unsupported"
	CodeRepair         = "repair"
	LocationFile       = "file"
	LocationLine       = "line"
	LocationRange      = "range"
	LocationLost       = "lost"
	AuthorHuman        = "human"
	AuthorAgent        = "agent"
	AuthorExternal     = "external"
	GitLabNote         = "note"
	GitLabSystemNote   = "system_note"
	RoleUser           = "user"
	RoleBot            = "bot"
	RoleExternal       = "external"
	ThreadOpenPrefix   = "[@th_"
	ThreadClosePrefix  = "[/@th_"
	MaxNameScalars     = 128
	MaxIdentityScalars = 256
	RevisionMissing    = "missing"
	UsageReasonMissing = "missing"
	OpThreadCreate     = "thread_create"
	OpThreadMove       = "thread_move"
	OpThreadDelete     = "thread_delete"
	OpMessageAdd       = "message_add"
	OpMessageUpdate    = "message_update"
	OpMessageDelete    = "message_delete"
)

type ValidateAuthorKindRequest struct{ Kind string }

func ValidateAuthorKind(r ValidateAuthorKindRequest) error {
	switch r.Kind {
	case AuthorHuman, AuthorAgent, AuthorExternal:
		return nil
	default:
		return invalid(invalidRequest{})
	}
}

type ValidateRevisionRequest struct{ Text string }
type HashRevisionRequest struct{ Bytes []byte }

var revisionPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func ValidateRevision(r ValidateRevisionRequest) error {
	if r.Text != RevisionMissing && !revisionPattern.MatchString(r.Text) {
		return invalid(invalidRequest{})
	}
	return nil
}
func HashRevision(r HashRevisionRequest) string {
	h := sha256.Sum256(r.Bytes)
	return "sha256:" + hex.EncodeToString(h[:])
}

type AuthorRoleRequest struct{ Kind string }

func AuthorRole(r AuthorRoleRequest) string {
	switch r.Kind {
	case AuthorHuman:
		return RoleUser
	case AuthorAgent:
		return RoleBot
	case AuthorExternal:
		return RoleExternal
	default:
		panic("invalid author kind")
	}
}

type RoleAuthorRequest struct{ Role string }

func RoleAuthor(r RoleAuthorRequest) string {
	switch r.Role {
	case RoleUser:
		return AuthorHuman
	case RoleBot:
		return AuthorAgent
	case RoleExternal:
		return AuthorExternal
	default:
		panic("invalid author role")
	}
}

type ReservedLineRequest struct{ Text string }

func ReservedLine(r ReservedLineRequest) bool {
	for _, p := range []string{ThreadOpenPrefix, ThreadClosePrefix, "---" + RoleUser, "---" + RoleBot, "---" + RoleExternal} {
		if strings.HasPrefix(r.Text, p) {
			return true
		}
	}
	return false
}

type LocationRankRequest struct{ Kind string }

func LocationRank(r LocationRankRequest) int {
	switch r.Kind {
	case LocationFile:
		return 0
	case LocationLine:
		return 1
	case LocationRange:
		return 2
	case LocationLost:
		return 3
	default:
		panic("invalid location kind")
	}
}

type ValidateNameRequest struct{ Text string }

func ValidateName(r ValidateNameRequest) error {
	if !validText(textRequest{Text: r.Text, Max: MaxNameScalars, NFC: true}) {
		return invalid(invalidRequest{})
	}
	return nil
}

type ValidateIdentityRequest struct{ Text string }

func ValidateIdentity(r ValidateIdentityRequest) error {
	if !validText(textRequest{Text: r.Text, Max: MaxIdentityScalars}) {
		return invalid(invalidRequest{})
	}
	return nil
}

type MessageMetadata struct {
	ID             string          `json:"id"`
	Name           *string         `json:"name,omitempty"`
	ExternalAuthor *ExternalAuthor `json:"external_author,omitempty"`
	GitLab         *GitLab         `json:"gitlab,omitempty"`
}
type ValidationDetails struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}
type IODetails struct {
	Operation              string `json:"operation"`
	Path                   string `json:"path"`
	CommitMayHaveSucceeded *bool  `json:"commit_may_have_succeeded,omitempty"`
}
type ConflictDetails struct {
	Kind string `json:"kind"`
}
type ConflictRequest struct{ Kind string }

func Conflict(r ConflictRequest) error {
	b, err := json.Marshal(ConflictDetails{Kind: r.Kind})
	if err != nil {
		panic(err)
	}
	return &Error{Code: CodeConflict, Message: CodeConflict + ":" + r.Kind, Details: b, Exit: ExitConflict}
}

type RevisionConflictDetails struct {
	Kind                   string  `json:"kind"`
	Expected               string  `json:"expected"`
	CurrentStorageRevision *string `json:"current_storage_revision"`
}
type RevisionConflictRequest struct {
	Expected               string
	CurrentStorageRevision *string
}

func RevisionConflict(r RevisionConflictRequest) error {
	b, err := json.Marshal(RevisionConflictDetails{Kind: "revision", Expected: r.Expected, CurrentStorageRevision: r.CurrentStorageRevision})
	if err != nil {
		panic(err)
	}
	return &Error{Code: CodeConflict, Message: CodeConflict + ":revision", Details: b, Exit: ExitConflict}
}

type RepairDetails struct {
	Diagnostics []Diagnostic `json:"diagnostics"`
}
