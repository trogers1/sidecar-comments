package runner

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// obligationLedgerV2 is deliberately a SHADOW ledger: it records a complete
// traceability target, not execution, conformance, coverage, or proof.
// ValidateArtifacts checks its coherence, but does not promote any entry.
type obligationLedgerV2 struct {
	LedgerVersion       int            `json:"ledger_version"`
	Mode                string         `json:"mode"`
	V1Inventory         string         `json:"v1_inventory"`
	RequiredRedRegistry string         `json:"required_red_registry"`
	Obligations         []obligationV2 `json:"obligations"`
}

type obligationV2 struct {
	ID               string   `json:"id"`
	Kind             string   `json:"kind"`
	LegacyID         string   `json:"legacy_id"`
	Aliases          []string `json:"aliases"`
	Status           string   `json:"status"`
	Reviewed         bool     `json:"reviewed"`
	Rationale        string   `json:"rationale"`
	NormativeSource  string   `json:"normative_source"`
	CoverageCategory string   `json:"coverage_category"`
}

type mandatoryOmissionV2 struct {
	Category string
	Source   string
}

// mandatoryOmissionsV2 is intentionally an explicit, granular coverage table.
// These are pending traceability targets, not evidence that their behavior ran
// or conforms.
var mandatoryOmissionsV2 = map[string]mandatoryOmissionV2{
	"BHV-RED-ARCHITECTURE-MODULE-BOUNDARIES":          {"architecture", "docs/v1/architecture.md#planned-module-and-packages"},
	"BHV-RED-ARCHITECTURE-TRUST-BOUNDARIES":           {"architecture", "docs/v1/architecture.md#trust-boundaries-and-code-principles"},
	"BHV-RED-ARCHITECTURE-AUTOMATED-BOUNDARY-CHECKS":  {"architecture", "docs/v1/architecture.md#automated-boundary-checks"},
	"BHV-RED-COMPATIBILITY-V1-ACCEPTANCE":             {"compatibility", "docs/v1/compatibility.md#compatibility-policy"},
	"BHV-RED-COMPATIBILITY-PROVISIONING-ENVIRONMENTS": {"compatibility", "docs/v1/compatibility.md#provisioning-and-supported-environments"},
	"BHV-RED-COMPATIBILITY-NO-EXTERNAL-CONFIGURATION": {"compatibility", "docs/v1/compatibility.md#no-inventory-or-external-configuration"},
	"BHV-RED-COMPATIBILITY-DURABILITY-RECOVERY":       {"compatibility", "docs/v1/compatibility.md#durability-recovery"},
	"BHV-RED-NEOVIM-PUBLIC-API-STATE":                 {"neovim", "docs/v1/neovim.md#public-lua-api-commands-and-state"},
	"BHV-RED-NEOVIM-BINARY-ELIGIBILITY":               {"neovim", "docs/v1/neovim.md#binary-resolution-and-eligibility"},
	"BHV-RED-NEOVIM-UI-ASYNC-CORRELATION":             {"neovim", "docs/v1/neovim.md#observable-ui-and-async-rules"},
	"BHV-RED-NEOVIM-FAIL-CLOSURE-DIRTY-BUFFERS":       {"neovim", "docs/v1/neovim.md#fail-closure-and-dirty-buffers"},
	"BHV-RED-BODY-TRANSPORT-FRAMING":                  {"body_transport", "docs/v1/format.md#framing-algorithm"},
	"BHV-RED-BODY-TRANSPORT-APPLY-LOGICAL-BYTES":      {"body_transport", "docs/v1/protocol.md#apply-request"},
	"BHV-RED-OUTPUT-JSON-ENVELOPES":                   {"output_results", "docs/v1/protocol.md#envelopes-and-output-mode"},
	"BHV-RED-OUTPUT-MODES":                            {"output_results", "docs/v1/protocol.md#envelopes-and-output-mode"},
	"BHV-RED-RESULTS-VARIANTS-CHANGED":                {"output_results", "docs/v1/protocol.md#dtos-and-result-variants"},
	"BHV-RED-STORAGE-WRITER-PLATFORM-GATE":            {"storage", "docs/v1/storage-security.md#supported-writer-platforms"},
	"BHV-RED-STORAGE-SECURE-OBJECT-CLASSIFICATION":    {"storage", "docs/v1/storage-security.md#secure-object-rules"},
	"BHV-RED-STORAGE-LOCK-FAILURE-CLASSIFICATION":     {"storage", "docs/v1/storage-security.md#secure-object-rules"},
	"BHV-RED-STORAGE-TRANSACTION-REVALIDATION":        {"storage", "docs/v1/storage-security.md#required-writer-transaction"},
	"BHV-RED-STORAGE-COMMIT-DURABILITY-FAILURES":      {"storage", "docs/v1/storage-security.md#atomic-replace-and-deletion"},
	"BHV-RED-SOURCE-OBJECT-ENDINGS":                   {"source", "docs/v1/source-and-location.md#source-validator"},
	"BHV-RED-SOURCE-RUNE-BOUNDARY-COORDINATES":        {"source", "docs/v1/source-and-location.md#source-validator"},
	"BHV-RED-SOURCE-ANCHOR-VALIDATION":                {"source", "docs/v1/source-and-location.md#public-location-failure-symbols"},
	"BHV-RED-SOURCE-LOCKED-REVALIDATION":              {"source", "docs/v1/source-and-location.md#when-validation-occurs"},
}

const obligationsV2SchemaURL = "https://github.com/trogers1/sidecar-comments/contracts/schema/obligations-v2.json"

func decodeObligationLedgerV2(path string) (obligationLedgerV2, error) {
	var ledger obligationLedgerV2
	data, err := readUnique(path) // rejects duplicate keys at every depth
	if err != nil {
		return ledger, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&ledger); err != nil {
		return ledger, fmt.Errorf("obligation ledger: %w", err)
	}
	if err = decoder.Decode(&struct{}{}); err != io.EOF {
		return ledger, fmt.Errorf("obligation ledger: trailing JSON")
	}
	return ledger, nil
}

// ValidateObligationLedgerV2 validates only the checked-in shadow ledger.
// Passing artifact validation is not execution or production conformance.
func ValidateObligationLedgerV2(base string) error {
	if base == "" {
		base = "."
	}
	ledgerPath := filepath.Join(base, "contracts/packs/obligations-v2.json")
	schemaPath := filepath.Join(base, "contracts/schema/obligations-v2.json")
	ledgerBytes, err := readUnique(ledgerPath)
	if err != nil {
		return err
	}
	schemaBytes, err := readUnique(schemaPath)
	if err != nil {
		return err
	}
	validator, err := compileSchemaValidator(obligationsV2SchemaURL, []schemaResource{{Location: obligationsV2SchemaURL, Source: schemaBytes}})
	if err != nil {
		return fmt.Errorf("obligation ledger schema: %w", err)
	}
	if err = validator.validate(ledgerBytes); err != nil {
		return fmt.Errorf("obligation ledger schema validation: %w", err)
	}
	ledger, err := decodeObligationLedgerV2(ledgerPath)
	if err != nil {
		return err
	}
	if ledger.LedgerVersion != 2 || ledger.Mode != "shadow_traceability_only" || ledger.V1Inventory != "contracts/behavior-inventory-v1.json" || ledger.RequiredRedRegistry != "contracts/packs/required-red-v1.json" {
		return fmt.Errorf("invalid shadow obligation ledger header")
	}
	inventory, err := decodeInventory(filepath.Join(base, ledger.V1Inventory))
	if err != nil {
		return err
	}
	registry, err := decodeRequiredRedRegistry(filepath.Join(base, ledger.RequiredRedRegistry))
	if err != nil {
		return err
	}
	requiredRed := make(map[string]bool, len(inventory.RequiredRed))
	for _, item := range inventory.RequiredRed {
		requiredRed[item.ID] = false
	}
	scenarios := make(map[string]bool, len(registry.Scenarios))
	for _, scenario := range registry.Scenarios {
		scenarios[scenario.ID] = false
	}
	ids, legacyIDs, aliases := map[string]bool{}, map[string]bool{}, map[string]bool{}
	seenExemptions := map[string]bool{}
	for _, item := range ledger.Obligations {
		if item.ID == "" || ids[item.ID] || strings.TrimSpace(item.Rationale) == "" {
			return fmt.Errorf("duplicate or incomplete shadow obligation %q", item.ID)
		}
		ids[item.ID] = true
		if err := validateObligationSource(base, item.NormativeSource); err != nil {
			return fmt.Errorf("%s: %w", item.ID, err)
		}
		if err := validateObligationApplicabilityV2(item); err != nil {
			return err
		}
		if item.Status == "not_applicable" {
			seenExemptions[item.LegacyID] = true
		}
		for _, alias := range item.Aliases {
			if alias == "" || aliases[alias] {
				return fmt.Errorf("%s: duplicate or empty legacy alias %q", item.ID, alias)
			}
			aliases[alias] = true
		}
		switch item.Kind {
		case "v1_required_red":
			if len(item.Aliases) != 1 || item.Aliases[0] != item.LegacyID {
				return fmt.Errorf("%s: legacy BHV alias is not retained", item.ID)
			}
			if item.LegacyID == "" || legacyIDs[item.LegacyID] {
				return fmt.Errorf("%s: duplicate or empty v1 required-red mapping", item.ID)
			}
			seen, ok := requiredRed[item.LegacyID]
			if !ok || seen {
				return fmt.Errorf("%s: unknown or repeated v1 required-red ID %q", item.ID, item.LegacyID)
			}
			requiredRed[item.LegacyID], legacyIDs[item.LegacyID] = true, true
		case "registered_pack_scenario":
			if len(item.Aliases) != 1 || item.Aliases[0] != item.LegacyID {
				return fmt.Errorf("%s: legacy P alias is not retained", item.ID)
			}
			if item.LegacyID == "" || legacyIDs[item.LegacyID] {
				return fmt.Errorf("%s: duplicate or empty pack-scenario mapping", item.ID)
			}
			seen, ok := scenarios[item.LegacyID]
			if !ok || seen {
				return fmt.Errorf("%s: unknown or repeated pack scenario %q", item.ID, item.LegacyID)
			}
			scenarios[item.LegacyID], legacyIDs[item.LegacyID] = true, true
		case "omission":
			if item.LegacyID != "" || len(item.Aliases) != 0 {
				return fmt.Errorf("%s: omission must not carry legacy identifiers", item.ID)
			}
		default:
			return fmt.Errorf("%s: invalid obligation kind %q", item.ID, item.Kind)
		}
		if item.Kind != "omission" && item.CoverageCategory != "" {
			return fmt.Errorf("%s: coverage category is reserved for omissions", item.ID)
		}
	}
	if len(seenExemptions) != len(requiredExemptionsV2) {
		return fmt.Errorf("shadow ledger lacks a required semantic-precondition exclusion")
	}
	for id, seen := range requiredRed {
		if !seen {
			return fmt.Errorf("unreconciled v1 required-red ID %s", id)
		}
	}
	for id, seen := range scenarios {
		if !seen {
			return fmt.Errorf("unreconciled registered pack scenario %s", id)
		}
	}
	return validateMandatoryOmissionsV2(ledger.Obligations)
}

// Only these four v1 requests describe a semantic precondition that the CLI
// explicitly does not define. The exact IDs and citation are enforced, not
// just the number of exclusions.
var requiredExemptionsV2 = map[string]bool{
	"BHV-RED-P4-THREAD-MOVE-PRECONDITION":    true,
	"BHV-RED-P4-THREAD-DELETE-PRECONDITION":  true,
	"BHV-RED-P4-MESSAGE-UPDATE-PRECONDITION": true,
	"BHV-RED-P4-MESSAGE-DELETE-PRECONDITION": true,
}

func validateObligationApplicabilityV2(item obligationV2) error {
	if item.Kind == "v1_required_red" && requiredExemptionsV2[item.LegacyID] {
		if item.Status != "not_applicable" || !item.Reviewed || item.NormativeSource != "docs/v1/cli.md" {
			return fmt.Errorf("%s: required semantic-precondition exclusion is missing or mis-cited", item.ID)
		}
		return nil
	}
	if item.Status != "pending" || item.Reviewed {
		return fmt.Errorf("%s: applicable obligation must remain pending", item.ID)
	}
	return nil
}

func validateMandatoryOmissionsV2(obligations []obligationV2) error {
	seen := make(map[string]bool, len(mandatoryOmissionsV2))
	for _, item := range obligations {
		if item.Kind != "omission" {
			continue
		}
		expected, known := mandatoryOmissionsV2[item.ID]
		if !known || seen[item.ID] {
			return fmt.Errorf("%s: invalid mandatory omission", item.ID)
		}
		if item.CoverageCategory != expected.Category {
			return fmt.Errorf("%s: coverage category %q, want %q", item.ID, item.CoverageCategory, expected.Category)
		}
		if item.NormativeSource != expected.Source {
			return fmt.Errorf("%s: normative source %q, want %q", item.ID, item.NormativeSource, expected.Source)
		}
		seen[item.ID] = true
	}
	for id := range mandatoryOmissionsV2 {
		if !seen[id] {
			return fmt.Errorf("missing mandatory omission %s", id)
		}
	}
	return nil
}

func validateObligationSource(base, reference string) error {
	path, fragment, hasFragment := strings.Cut(reference, "#")
	if !safeReference(path) || !(strings.HasPrefix(path, "docs/v1/") || strings.HasPrefix(path, "contracts/packs/")) {
		return fmt.Errorf("unsafe or non-normative source %q", reference)
	}
	if hasFragment && (fragment == "" || strings.ContainsAny(fragment, "\\\\ \t\r\n")) {
		return fmt.Errorf("malformed normative source reference %q", reference)
	}
	info, err := os.Stat(filepath.Join(base, path))
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("stale normative source %q", reference)
	}
	return nil
}
