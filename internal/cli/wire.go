package cli

import (
	"encoding/json"
	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/operations"
)

type usageDetails struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}
type errorValue struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details json.RawMessage `json:"details"`
}
type errorEnvelope struct {
	Protocol int        `json:"protocol"`
	Error    errorValue `json:"error"`
}
type successEnvelope struct {
	Protocol int             `json:"protocol"`
	Command  string          `json:"command"`
	Changed  bool            `json:"changed"`
	Result   json.RawMessage `json:"result"`
}
type versionResult struct {
	CLIVersion    string `json:"cli_version"`
	FormatVersion int    `json:"format_version"`
	Protocol      int    `json:"protocol"`
}
type repairDetails = domain.RepairDetails
type invalidResult struct {
	Valid  bool          `json:"valid"`
	Repair repairDetails `json:"repair"`
	revision
}
type validResult struct {
	Valid    bool      `json:"valid"`
	Warnings []warning `json:"warnings"`
	revision
}
type listResult struct {
	Threads  []domain.Thread `json:"threads"`
	Warnings []warning       `json:"warnings"`
	revision
}
type notFoundDetails struct {
	Entity string `json:"entity"`
	ID     string `json:"id"`
}
type assignmentsResult struct {
	Assignments []assignment `json:"assignments"`
	revision
}
type formatResult struct {
	Formatted bool `json:"formatted"`
	revision
}
type applyResult struct {
	Operations []operationResult `json:"operations"`
	revision
}
type unsupportedDetails struct {
	Received string `json:"received"`
}
type wireDTO interface {
	domain.RepairDetails | operations.PreconditionDetails | result | usageDetails | errorValue | errorEnvelope | successEnvelope | versionResult | invalidResult | validResult | listResult | notFoundDetails | assignmentsResult | formatResult | applyResult | unsupportedDetails
}
