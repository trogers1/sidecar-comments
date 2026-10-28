package operations

import (
	"github.com/trogers1/sidecar-comments/internal/domain"
)

// Value is immutable and can only be populated by NewValue.
type Value struct {
	dto       OperationDTO
	threadID  domain.ThreadID
	messageID domain.MessageID
	valid     bool
}
type ValueRequest struct{}

func (v Value) Op(r ValueRequest) string { return v.dto.Op }
func (v Value) Location(r ValueRequest) domain.Location {
	return domain.CloneLocation(domain.CloneLocationRequest{Location: v.dto.Location})
}

type NewValueRequest struct{ DTO OperationDTO }
type ValidateOperationFieldsRequest struct {
	DTO             OperationDTO
	AllowMissingIDs bool
}

func ValidateOperationFields(r ValidateOperationFieldsRequest) error {
	o := r.DTO
	invalid := func(r invalidOperationRequest) error {
		return domain.Validation(domain.ValidationRequest{Path: "target_content", Reason: "invalid"})
	}
	if !r.AllowMissingIDs || o.ThreadID != "" || o.Op != domain.OpThreadCreate {
		if err := domain.ValidateID(domain.ValidateIDRequest{ID: o.ThreadID}); err != nil {
			return err
		}
	}
	needsMessage := false
	switch o.Op {
	case domain.OpThreadCreate, domain.OpMessageAdd:
		needsMessage = true
		if err := domain.ValidateAuthor(domain.ValidateAuthorRequest{Author: o.Author}); err != nil {
			return &FieldError{Field: "argv", Cause: err}
		}
		if err := domain.ValidateGitLab(domain.ValidateGitLabRequest{GitLab: o.GitLab}); err != nil {
			return &FieldError{Field: "metadata", Cause: err}
		}
		if err := domain.ValidateBody(domain.ValidateBodyRequest{Body: o.Body}); err != nil {
			return &FieldError{Field: "body", Cause: err}
		}
	case domain.OpMessageUpdate:
		needsMessage = true
		if err := domain.ValidateBody(domain.ValidateBodyRequest{Body: o.Body}); err != nil {
			return &FieldError{Field: "body", Cause: err}
		}
	case domain.OpMessageDelete:
		needsMessage = true
	case domain.OpThreadMove, domain.OpThreadDelete:
	default:
		return invalid(invalidOperationRequest{})
	}
	if needsMessage {
		if !r.AllowMissingIDs || o.MessageID != "" || (o.Op != domain.OpThreadCreate && o.Op != domain.OpMessageAdd) {
			if err := domain.ValidateID(domain.ValidateIDRequest{ID: o.MessageID}); err != nil {
				return err
			}
		}
	} else if o.MessageID != "" {
		return invalid(invalidOperationRequest{})
	}
	if o.Op == domain.OpThreadCreate || o.Op == domain.OpThreadMove {
		if err := domain.ValidateLocation(domain.ValidateLocationRequest{Location: o.Location}); err != nil {
			return err
		}
	} else if !equal(equalRequest[domain.Location]{Left: o.Location, Right: domain.Location{}}) {
		return invalid(invalidOperationRequest{})
	}
	if o.Op != domain.OpThreadCreate && o.Op != domain.OpMessageAdd {
		if o.Author.Kind != "" || o.Author.Name != nil || o.Author.ExternalAuthor != nil || o.GitLab != nil {
			return invalid(invalidOperationRequest{})
		}
	}
	if o.Op != domain.OpThreadCreate && o.Op != domain.OpMessageAdd && o.Op != domain.OpMessageUpdate && o.Body != "" {
		return invalid(invalidOperationRequest{})
	}
	if o.IfLastAuthorKind != "" {
		if o.Op != domain.OpMessageAdd || domain.ValidateAuthorKind(domain.ValidateAuthorKindRequest{Kind: o.IfLastAuthorKind}) != nil {
			return invalid(invalidOperationRequest{})
		}
	}
	return nil
}
func NewValue(r NewValueRequest) (Value, error) {
	if err := ValidateOperationFields(ValidateOperationFieldsRequest{DTO: r.DTO}); err != nil {
		return Value{}, err
	}
	dto := detachOperation(detachOperationRequest{DTO: r.DTO})
	tid, err := domain.NewThreadID(domain.NewThreadIDRequest{Text: dto.ThreadID})
	if err != nil {
		return Value{}, err
	}
	var mid domain.MessageID
	if dto.MessageID != "" {
		mid, err = domain.NewMessageID(domain.NewMessageIDRequest{Text: dto.MessageID})
		if err != nil {
			return Value{}, err
		}
	}
	return Value{dto: dto, threadID: tid, messageID: mid, valid: true}, nil
}

type FieldError struct {
	Field string
	Cause error
}

func (e *FieldError) Error() string { return e.Cause.Error() }
func (e *FieldError) Unwrap() error { return e.Cause }

type invalidOperationRequest struct{}
type detachOperationRequest struct{ DTO OperationDTO }

// Detach pointer-bearing fields so neither inputs nor output DTOs expose Value.
func detachOperation(r detachOperationRequest) OperationDTO {
	dto := r.DTO
	dto.Location = domain.CloneLocation(domain.CloneLocationRequest{Location: dto.Location})
	dto.Author = domain.CloneAuthor(domain.CloneAuthorRequest{Author: dto.Author})
	dto.GitLab = domain.CloneGitLab(domain.CloneGitLabRequest{GitLab: dto.GitLab})
	return dto
}
