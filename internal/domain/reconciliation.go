package domain

// TrackingOutcome deliberately excludes retained source excerpts.
type TrackingOutcome struct {
	ThreadID string   `json:"thread_id"`
	Outcome  string   `json:"outcome"`
	Reason   string   `json:"reason"`
	Evidence string   `json:"evidence"`
	Location Location `json:"location"`
}

const (
	OutcomeUnchanged                = "unchanged"
	OutcomeMoved                    = "moved"
	OutcomeModified                 = "modified"
	OutcomeUnresolved               = "unresolved"
	OutcomeSkipped                  = "skipped"
	ReasonSourceUnchanged           = "source_unchanged"
	ReasonExactMatch                = "exact_match"
	ReasonContextReplacement        = "context_replacement"
	ReasonBoundaryExpansion         = "boundary_expansion"
	ReasonEditorLocation            = "editor_location"
	ReasonEditorDeleted             = "editor_deleted"
	ReasonNoMatch                   = "no_match"
	ReasonAmbiguous                 = "ambiguous"
	ReasonInsufficientContext       = "insufficient_context"
	ReasonUnsupportedTransformation = "unsupported_transformation"
	ReasonSelectionTooLarge         = "selection_too_large"
	ReasonSearchLimit               = "search_limit"
	ReasonFile                      = "file"
	ReasonLost                      = "lost"
	EvidenceUnchanged               = "unchanged"
	EvidenceExact                   = "exact"
	EvidenceInferred                = "inferred"
	EvidenceEditor                  = "editor"
	EvidenceNone                    = "none"
)
