package runner

import "testing"

func TestDeclarativePackGoldensRejectOpaqueObjects(t *testing.T) {
	if err := validateP8P10Golden([]byte(`{}`), "opaque.json", "P8-TEST"); err == nil {
		t.Fatal("opaque P8/P10 declarative golden accepted")
	}
	if err := validateP11P12DeclarativeGolden([]byte(`{}`), "opaque.json", "P12-TEST"); err == nil {
		t.Fatal("opaque P11/P12 declarative golden accepted")
	}
}

func TestMarkdownAnchorResolvesTableRow(t *testing.T) {
	row, err := markdownTableRow([]byte("| ID | mode |\n|---|---|\n| P12-10 | controlled:large_input |\n"), "p12-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(row) != 2 || row[0] != "P12-10" || row[1] != "controlled:large_input" {
		t.Fatalf("wrong row: %#v", row)
	}
	if _, err := markdownTableRow([]byte("| ID | mode |\n|---|---|\n| P12-10 | controlled |\n"), "p12-11"); err == nil {
		t.Fatal("missing anchor row accepted")
	}
}

func TestP8P10DeclarativeGoldenIsClosedAndBoundToScenario(t *testing.T) {
	valid := []byte(`{"case":"P8-TEST","expect":{"status":3,"error":"validation"}}`)
	if err := validateP8P10Golden(valid, "test.json", "P8-TEST"); err != nil {
		t.Fatalf("valid declarative golden rejected: %v", err)
	}
	if err := validateP8P10Golden([]byte(`{"case":"P8-TEST","expect":{"status":3},"extra":true}`), "test.json", "P8-TEST"); err == nil {
		t.Fatal("unknown declarative golden member accepted")
	}
	if err := validateP8P10Golden(valid, "test.json", "P8-OTHER"); err == nil {
		t.Fatal("mismatched scenario ID accepted")
	}
}
