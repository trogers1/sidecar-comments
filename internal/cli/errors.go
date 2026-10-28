package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/operations"
)

type runCleanupRequest struct{}
type inputCleanupRequest struct{}
type commandErrorRequest struct {
	Error   error
	Command string
}

// The executor uses canonical operation tags. Only this CLI boundary restores
// the public command spelling in a single-command precondition diagnostic.
func commandError(r commandErrorRequest) error {
	var e *domain.Error
	if !errors.As(r.Error, &e) || e.Code != domain.CodePrecondition {
		return r.Error
	}
	var details operations.PreconditionDetails
	if err := json.Unmarshal(e.Details, &details); err != nil {
		return fmt.Errorf("decode precondition details: %w", err)
	}
	details.Operation = r.Command
	return &domain.Error{Code: e.Code, Message: domain.CodePrecondition + ":" + r.Command + ":" + details.Reason, Exit: e.Exit, Details: raw(rawRequest[operations.PreconditionDetails]{Value: details})}
}
