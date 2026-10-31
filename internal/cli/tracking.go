package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/format"
	"github.com/trogers1/sidecar-comments/internal/operations"
	"github.com/trogers1/sidecar-comments/internal/store"
)

type syncResult struct {
	Check          bool    `json:"check"`
	WouldChange    bool    `json:"would_change"`
	SourceRevision string  `json:"source_revision,omitempty"`
	Previous       *string `json:"previous_storage_revision"`
	revision
	Outcomes []domain.TrackingOutcome `json:"outcomes"`
}
type editorResult struct {
	SourceRevision string `json:"source_revision"`
	revision
	Outcomes []domain.TrackingOutcome `json:"outcomes"`
}

func executeTracking(r execRequest) (out outcome, err error) {
	c := r.Config
	var editor domain.EditorRequest
	if c.command == "editor-update" {
		b, e := input(inputRequest{Path: c.operations, Field: "operations", Limit: domain.MaxApplyBytes, Stdin: r.Stdin})
		if e != nil {
			return out, e
		}
		if domain.ValidateDTO(domain.ValidateDTORequest{Bytes: b, Kind: "editor"}) != nil {
			return out, usage(usageRequest{Field: "operations", Reason: "malformed"})
		}
		d := json.NewDecoder(bytes.NewReader(b))
		d.DisallowUnknownFields()
		if d.Decode(&editor) != nil {
			return out, usage(usageRequest{Field: "operations", Reason: "malformed"})
		}
		if editor.Protocol != domain.ProtocolVersion {
			return out, domain.Unsupported(domain.UnsupportedRequest{Received: "protocol"})
		}
		if c.revision != "" && c.revision != editor.IfRevision {
			return out, usage(usageRequest{Field: "revision", Reason: "incompatible"})
		}
		c.revision = editor.IfRevision
		c.sourceRevision = editor.SourceRevision
	}
	if !c.check {
		if e := store.CheckWriterCapability(store.CheckWriterCapabilityRequest{}); e != nil {
			return out, e
		}
	}
	snap, e := store.Read(store.ReadRequest{Path: c.file, Kind: store.ReadTarget})
	if e != nil {
		return out, e
	}
	var source *domain.SourceValue
	load := func(r sourceRequest) error {
		path, e := store.DerivedSourcePath(store.DerivedSourcePathRequest{TargetPath: c.file})
		if e != nil {
			return e
		}
		s, e := store.ReadSourceValue(store.ReadSourceValueRequest{Path: path})
		if e != nil {
			return e
		}
		if c.sourceRevision != "" && c.sourceRevision != s.Fingerprint(domain.ValueRequest{}) {
			return domain.SourceRevisionConflict(domain.SourceRevisionConflictRequest{Expected: c.sourceRevision, CurrentSourceRevision: s.Fingerprint(domain.ValueRequest{})})
		}
		source = &s
		return nil
	}
	if c.command == "editor-update" {
		if e = load(sourceRequest{}); e != nil {
			return out, e
		}
	}
	var tx *store.Transaction
	if !c.check {
		begin := r.Begin
		if begin == nil {
			begin = store.Begin
		}
		tx, e = begin(store.BeginRequest{Path: c.file, Timeout: c.timeout})
		if e != nil {
			return out, e
		}
		defer func(r transactionCleanupRequest) {
			if e := tx.Close(store.TransactionCloseRequest{}); err == nil && e != nil {
				err = e
			}
		}(transactionCleanupRequest{})
		snap, e = tx.Read(store.TransactionReadRequest{})
		if e != nil {
			return out, e
		}
	}
	if c.revision != "" && ((c.revision == domain.RevisionMissing && snap.Exists) || (c.revision != domain.RevisionMissing && (snap.Revision == nil || *snap.Revision != c.revision))) {
		return out, domain.RevisionConflict(domain.RevisionConflictRequest{Expected: c.revision, CurrentStorageRevision: snap.Revision})
	}
	parsed, e := format.Parse(format.ParseRequest{Bytes: snap.Bytes, Mode: "normal"})
	if e != nil {
		return out, e
	}
	original := format.Encode(format.EncodeRequest{Document: parsed.Value})
	required := c.command == "editor-update"
	for _, t := range parsed.Value.Entries(domain.DocumentEntriesRequest{}) {
		required = required || t.Tracking(domain.ValueRequest{}) != nil
	}
	if required {
		if e = load(sourceRequest{}); e != nil {
			return out, e
		}
	}
	outcomes := []domain.TrackingOutcome{}
	if c.command == "sync" {
		response, e := operations.Reconcile(operations.ReconcileRequest{Document: parsed.Value, Source: source})
		if e != nil {
			return out, e
		}
		parsed.Value = response.Document
		outcomes = response.Outcomes
	} else {
		seen := map[string]bool{}
		threads := parsed.Value.Entries(domain.DocumentEntriesRequest{})
		for i, u := range editor.Updates {
			fail := func(r editorFailureRequest) error {
				return &domain.Error{Code: domain.CodePrecondition, Message: "precondition:editor-update:" + r.Reason, Exit: domain.ExitPrecondition, Details: raw(rawRequest[operations.PreconditionDetails]{Value: operations.PreconditionDetails{Operation: c.command, Reason: r.Reason, Index: &i}})}
			}
			if seen[u.ThreadID] {
				return out, fail(editorFailureRequest{Reason: "duplicate_thread"})
			}
			seen[u.ThreadID] = true
			found := -1
			for j, t := range threads {
				if t.DTO(domain.ValueRequest{}).ID == u.ThreadID {
					found = j
					break
				}
			}
			if found < 0 {
				return out, editorNotFound(editorNotFoundRequest{ID: u.ThreadID, Index: i})
			}
			old := threads[found]
			loc := domain.Location{Kind: domain.LocationUnresolved}
			tracking := old.Tracking(domain.ValueRequest{})
			result := domain.TrackingOutcome{ThreadID: u.ThreadID, Outcome: domain.OutcomeUnresolved, Reason: domain.ReasonEditorDeleted, Evidence: domain.EvidenceEditor, Location: loc}
			if u.Deleted != nil {
				if tracking == nil {
					return out, fail(editorFailureRequest{Reason: "not_tracked"})
				}
			} else {
				loc = *u.Location
				tracking, e = captureOperation(captureOperationRequest{Document: parsed.Value, DTO: operations.OperationDTO{Op: domain.OpThreadMove, Location: loc}, Source: source, Command: c.command, Index: &i})
				if e != nil {
					return out, e
				}
				result.Location = loc
				result.Reason = domain.ReasonEditorLocation
				result.Outcome = domain.OutcomeMoved
				previous := old.Tracking(domain.ValueRequest{})
				if previous != nil && previous.SelectedText != tracking.SelectedText {
					result.Outcome = domain.OutcomeModified
				} else if old.Location(domain.ValueRequest{}) == loc {
					result.Outcome = domain.OutcomeUnchanged
				}
			}
			t, e := domain.NewThreadValue(domain.NewThreadValueRequest{ID: old.ID(domain.ValueRequest{}), Location: loc, Messages: old.Messages(domain.ValueRequest{}), Tracking: tracking})
			if e != nil {
				return out, e
			}
			threads[found] = t
			outcomes = append(outcomes, result)
		}
		parsed.Value, e = domain.NewDocumentFromValues(domain.NewDocumentFromValuesRequest{Threads: threads})
		if e != nil {
			return out, trackingMutationError(trackingMutationErrorRequest{Error: e, Operation: c.command})
		}
	}
	canonical := format.Encode(format.EncodeRequest{Document: parsed.Value})
	if len(canonical) > domain.MaxTargetBytes {
		return out, domain.Validation(domain.ValidationRequest{Path: "target_size", Reason: "too_large"})
	}
	different := !bytes.Equal(canonical, snap.Bytes) || snap.Exists && len(canonical) == 0
	fingerprint := ""
	if source != nil {
		fingerprint = source.Fingerprint(domain.ValueRequest{})
	}
	validate := func(store.CommitValidationRequest) error {
		if !required {
			return nil
		}
		if e := load(sourceRequest{}); e != nil {
			return e
		}
		if source.Fingerprint(domain.ValueRequest{}) != fingerprint {
			return domain.SourceRevisionConflict(domain.SourceRevisionConflictRequest{Expected: fingerprint, CurrentSourceRevision: source.Fingerprint(domain.ValueRequest{})})
		}
		return nil
	}
	cr := hash(hashRequest{Bytes: canonical})
	rev := revision{Storage: snap.Revision, Canonical: &cr}
	if c.check {
		cr = hash(hashRequest{Bytes: original})
		if different {
			out.Exit = domain.ExitValidation
		}
		for _, o := range outcomes {
			if o.Outcome == domain.OutcomeUnresolved {
				out.Exit = domain.ExitValidation
			}
		}
	} else if different {
		if e = tx.Commit(store.CommitRequest{Bytes: canonical, Validate: validate}); e != nil {
			return out, e
		}
		out.Changed = true
		if len(canonical) == 0 {
			rev.Storage = nil
		} else {
			v := hash(hashRequest{Bytes: canonical})
			rev.Storage = &v
		}
	}
	if c.command == "sync" {
		out.Bytes = raw(rawRequest[syncResult]{Value: syncResult{Check: c.check, WouldChange: different, SourceRevision: fingerprint, Previous: snap.Revision, revision: rev, Outcomes: outcomes}})
	} else {
		out.Bytes = raw(rawRequest[editorResult]{Value: editorResult{SourceRevision: fingerprint, revision: rev, Outcomes: outcomes}})
	}
	return out, nil
}

type trackingMutationErrorRequest struct {
	Error     error
	Operation string
}

func trackingMutationError(r trackingMutationErrorRequest) error {
	var typed *domain.Error
	if !errors.As(r.Error, &typed) || typed.Code != domain.CodeValidation {
		return r.Error
	}
	var details domain.ValidationDetails
	if err := json.Unmarshal(typed.Details, &details); err != nil {
		return r.Error
	}
	if details.Path != "tracking" || details.Reason != "too_large" {
		return r.Error
	}
	return &domain.Error{Code: domain.CodePrecondition, Message: "precondition:" + r.Operation + ":limit_tracking", Exit: domain.ExitPrecondition, Details: raw(rawRequest[operations.PreconditionDetails]{Value: operations.PreconditionDetails{Operation: r.Operation, Reason: "limit_tracking"}})}
}

type editorFailureRequest struct{ Reason string }

type editorNotFoundRequest struct {
	ID    string
	Index int
}

func editorNotFound(r editorNotFoundRequest) error {
	b, err := json.Marshal(struct {
		Entity string `json:"entity"`
		ID     string `json:"id"`
		Index  int    `json:"index"`
	}{"thread", r.ID, r.Index})
	if err != nil {
		panic(err)
	}
	return &domain.Error{Code: domain.CodeNotFound, Message: "not_found:thread", Details: b, Exit: domain.ExitNotFound}
}
