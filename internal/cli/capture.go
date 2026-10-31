package cli

import (
	"errors"
	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/operations"
)

type existingCreateRequest struct {
	Document domain.DocumentValue
	ID       string
}

func existingCreate(r existingCreateRequest) bool {
	for _, t := range r.Document.Entries(domain.DocumentEntriesRequest{}) {
		if t.DTO(domain.ValueRequest{}).ID == r.ID {
			return true
		}
	}
	return false
}

type captureOperationRequest struct {
	Document domain.DocumentValue
	DTO      operations.OperationDTO
	Source   *domain.SourceValue
	Command  string
	Index    *int
}

func captureOperation(r captureOperationRequest) (*domain.TrackingRecord, error) {
	o := r.DTO
	if o.Op != domain.OpThreadCreate && o.Op != domain.OpThreadMove {
		return nil, nil
	}
	if o.Op == domain.OpThreadCreate && o.ThreadID != "" {
		for _, t := range r.Document.Entries(domain.DocumentEntriesRequest{}) {
			if t.DTO(domain.ValueRequest{}).ID == o.ThreadID {
				// Execute checks exact retry equality before consulting capture evidence.
				return nil, nil
			}
		}
	}
	if o.Location.Kind == domain.LocationLost {
		return nil, nil
	}
	if r.Source == nil {
		return nil, domain.Validation(domain.ValidationRequest{Path: "source_coordinate", Reason: "location"})
	}
	if err := r.Source.ValidateLocation(domain.SourceLocationRequest{Location: o.Location}); err != nil {
		return nil, err
	}
	if o.Location.Kind == domain.LocationFile {
		return nil, nil
	}
	tracking, err := r.Source.Capture(domain.SourceLocationRequest{Location: o.Location})
	var limit *domain.CaptureLimitError
	if errors.As(err, &limit) {
		return nil, &domain.Error{Code: domain.CodePrecondition, Message: "precondition:" + r.Command + ":limit_selection", Exit: domain.ExitPrecondition, Details: raw(rawRequest[operations.PreconditionDetails]{Value: operations.PreconditionDetails{Operation: r.Command, Reason: "limit_selection", Index: r.Index}})}
	}
	if err != nil {
		return nil, err
	}
	return &tracking, nil
}

// Derive source requirements in request order, including retries after earlier
// creates and fresh creates after deletion. This does not execute mutations.
type applySourceLocationsRequest struct {
	Document   domain.DocumentValue
	Operations []operations.Value
}

func applySourceLocations(r applySourceLocationsRequest) []domain.Location {
	messages := map[string]map[string]bool{}
	for _, t := range r.Document.Entries(domain.DocumentEntriesRequest{}) {
		dto := t.DTO(domain.ValueRequest{})
		ids := map[string]bool{}
		for _, m := range dto.Messages {
			ids[m.ID] = true
		}
		messages[dto.ID] = ids
	}
	locations := []domain.Location{}
	for _, v := range r.Operations {
		o := v.DTO(operations.ValueRequest{})
		switch o.Op {
		case domain.OpThreadCreate:
			if _, exists := messages[o.ThreadID]; !exists {
				locations = append(locations, o.Location)
				messages[o.ThreadID] = map[string]bool{o.MessageID: true}
			}
		case domain.OpThreadMove:
			locations = append(locations, o.Location)
		case domain.OpThreadDelete:
			delete(messages, o.ThreadID)
		case domain.OpMessageAdd:
			if ids, exists := messages[o.ThreadID]; exists {
				ids[o.MessageID] = true
			}
		case domain.OpMessageDelete:
			if ids, exists := messages[o.ThreadID]; exists {
				delete(ids, o.MessageID)
				if len(ids) == 0 {
					delete(messages, o.ThreadID)
				}
			}
		}
	}
	return locations
}

type trackingListResult struct {
	Threads  []domain.Thread `json:"threads"`
	Warnings []warning       `json:"warnings"`
	revision
}
type trackingGetResult struct {
	Thread domain.Thread `json:"thread"`
	revision
}
