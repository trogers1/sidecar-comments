package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestObligationLedgerV2IsCompleteShadowTraceability(t *testing.T) {
	root := repoRootForTest(t)
	if err := ValidateObligationLedgerV2(root); err != nil {
		t.Fatalf("shadow obligation ledger: %v", err)
	}
	ledger, err := decodeObligationLedgerV2(filepath.Join(root, "contracts/packs/obligations-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	if ledger.Mode != "shadow_traceability_only" {
		t.Fatalf("mode = %q", ledger.Mode)
	}
	notApplicable := 0
	for _, item := range ledger.Obligations {
		if item.Status == "not_applicable" {
			notApplicable++
		}
	}
	if notApplicable != 4 {
		t.Fatalf("reviewed not_applicable entries = %d, want 4", notApplicable)
	}
	if err := validateMandatoryOmissionsV2(ledger.Obligations); err != nil {
		t.Fatalf("mandatory omissions: %v", err)
	}
}

func TestObligationLedgerV2RejectsSwappedExemptionAtSameCount(t *testing.T) {
	root := repoRootForTest(t)
	ledger, err := decodeObligationLedgerV2(filepath.Join(root, "contracts/packs/obligations-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var exempt, applicable obligationV2
	for _, item := range ledger.Obligations {
		switch item.LegacyID {
		case "BHV-RED-P4-THREAD-MOVE-PRECONDITION":
			exempt = item
		case "P6-POST-LOCK-SOURCE-REPLACEMENT":
			applicable = item
		}
	}
	if exempt.ID == "" || applicable.ID == "" {
		t.Fatal("test obligations missing")
	}
	exempt.Status, exempt.Reviewed = "pending", false
	applicable.Status, applicable.Reviewed = "not_applicable", true
	for _, item := range []obligationV2{exempt, applicable} {
		if err := validateObligationApplicabilityV2(item); err == nil {
			t.Fatalf("swapped exemption accepted for %s", item.ID)
		}
	}
}

func TestObligationLedgerV2RequiresEveryMandatoryOmissionCategory(t *testing.T) {
	root := repoRootForTest(t)
	ledger, err := decodeObligationLedgerV2(filepath.Join(root, "contracts/packs/obligations-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	categories := map[string]bool{}
	for _, expected := range mandatoryOmissionsV2 {
		categories[expected.Category] = true
	}
	for category := range categories {
		t.Run(category, func(t *testing.T) {
			withoutCategory := make([]obligationV2, 0, len(ledger.Obligations))
			for _, item := range ledger.Obligations {
				if item.Kind == "omission" && item.CoverageCategory == category {
					continue
				}
				withoutCategory = append(withoutCategory, item)
			}
			if err := validateMandatoryOmissionsV2(withoutCategory); err == nil {
				t.Fatalf("removing mandatory %q omissions was accepted", category)
			}
		})
	}
}

func TestObligationLedgerV2RejectsMalformedNormativeReferences(t *testing.T) {
	root := repoRootForTest(t)
	for _, reference := range []string{
		"../docs/v1/architecture.md",
		"docs/v1/architecture.md#",
		"docs/v1/architecture.md#bad fragment",
		"docs/v1/does-not-exist.md",
		"README.md",
	} {
		t.Run(reference, func(t *testing.T) {
			if err := validateObligationSource(root, reference); err == nil {
				t.Fatalf("malformed reference %q was accepted", reference)
			}
		})
	}
}

func TestObligationLedgerV2RejectsDuplicateAndUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	duplicate := filepath.Join(dir, "duplicate.json")
	if err := os.WriteFile(duplicate, []byte(`{"ledger_version":2,"ledger_version":2}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeObligationLedgerV2(duplicate); err == nil {
		t.Fatal("duplicate ledger key accepted")
	}
	unknown := filepath.Join(dir, "unknown.json")
	data := []byte(`{"ledger_version":2,"mode":"shadow_traceability_only","v1_inventory":"contracts/behavior-inventory-v1.json","required_red_registry":"contracts/packs/required-red-v1.json","obligations":[],"proof":true}`)
	if err := os.WriteFile(unknown, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeObligationLedgerV2(unknown); err == nil {
		t.Fatal("unknown ledger key accepted")
	}
}
