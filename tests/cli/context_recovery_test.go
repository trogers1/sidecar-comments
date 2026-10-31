package cli_test

import (
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

func TestSubjectContextAutomaticRecovery(t *testing.T) {
	const source = "prefix chosen suffix\n"
	f := newContextFixture(contextFixtureRequest{T: t, Source: source, Location: "range:1:8-1:14"})
	original := f.read(contextReadRequest{})
	originalFiles := f.snapshot(contextReadRequest{})
	f.source(contextSourceRequest{Text: "replacement without the retained selection\n"})
	unresolved := f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}})
	outcomes := contextOutcomes(contextOutcomesRequest{T: t, Response: unresolved})
	if !unresolved.Changed || len(outcomes.Outcomes) != 1 || outcomes.Outcomes[0].Outcome != domain.OutcomeUnresolved {
		t.Fatalf("expected unresolved attachment: %+v", outcomes)
	}
	retained := f.read(contextReadRequest{})
	if *retained.Thread.Tracking != *original.Thread.Tracking {
		t.Fatal("unresolved state changed retained evidence")
	}
	f.source(contextSourceRequest{Text: source})
	recovered := f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}})
	outcomes = contextOutcomes(contextOutcomesRequest{T: t, Response: recovered})
	if !recovered.Changed || len(outcomes.Outcomes) != 1 || outcomes.Outcomes[0].Outcome != domain.OutcomeMoved || outcomes.Outcomes[0].Evidence != domain.EvidenceExact {
		t.Fatalf("unique restored evidence did not recover: %+v", outcomes)
	}
	current := f.read(contextReadRequest{})
	if current.Thread.Location != original.Thread.Location || *current.Thread.Tracking != *original.Thread.Tracking || current.StorageRevision != original.StorageRevision {
		t.Fatal("recovery failed to restore the original attachment and conversation")
	}
	if err := compareFiles(filesExpectation{Got: f.snapshot(contextReadRequest{}), Want: originalFiles}); err != nil {
		t.Fatal(err)
	}
	if f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget, "--check"}}).Changed {
		t.Fatal("recovered attachment is not stable")
	}
}
