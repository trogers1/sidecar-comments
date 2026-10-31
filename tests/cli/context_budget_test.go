package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

func TestSubjectContextLaterEmptyThreadHonorsGlobalBudget(t *testing.T) {
	f := contextFixture{t: t, binary: subjectBinary(subjectRequest{T: t}), dir: t.TempDir()}
	const selection = "needle"
	f.source(contextSourceRequest{Text: selection})
	consumers := domain.MaxSyncWorkBytes / domain.MaxSourceBytes
	ids := make([]struct{ thread, message string }, consumers+1)
	for i := range ids {
		var err error
		ids[i].thread, err = domain.NewID(domain.NewIDRequest{})
		if err != nil {
			t.Fatal(err)
		}
		ids[i].message, err = domain.NewID(domain.NewIDRequest{})
		if err != nil {
			t.Fatal(err)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].thread < ids[j].thread })
	for i, pair := range ids {
		location := fmt.Sprintf("range:1:1-1:%d", len(selection)+1)
		if i == consumers {
			f.source(contextSourceRequest{Text: ""})
			location = "line:1"
		}
		f.call(contextCallRequest{Args: []string{"thread", "create", "--file", contextTarget, "--location", location, "--author-kind", domain.AuthorHuman, "--body-file", "-", "--id", pair.thread, "--message-id", pair.message}, Stdin: []byte("Preserve the conversation.")})
	}
	last := ids[len(ids)-1]
	read := func(r contextReadRequest) contextThreadResult {
		response := f.call(contextCallRequest{Args: []string{"thread", "get", "--file", contextTarget, "--id", last.thread, "--include-tracking"}})
		var result contextThreadResult
		if err := json.Unmarshal(response.Result, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := read(contextReadRequest{})
	// Each needle search requires preprocessing plus a complete source pass;
	// its per-thread budget runs out before uniqueness can be established.
	// Together they consume the global cap before the empty-line record runs.
	source := strings.Repeat("a", domain.MaxSourceBytes)
	f.source(contextSourceRequest{Text: source})
	expectedSource := domain.HashRevision(domain.HashRevisionRequest{Bytes: []byte(source)})
	result := f.call(contextCallRequest{Args: []string{"sync", "--file", contextTarget}, Timeout: 60 * time.Second})
	outcomes := contextOutcomes(contextOutcomesRequest{T: t, Response: result})
	if !result.Changed || len(outcomes.Outcomes) != len(ids) {
		t.Fatalf("missing outcomes: %+v", outcomes)
	}
	for i, outcome := range outcomes.Outcomes {
		if outcome.ThreadID != ids[i].thread || outcome.Outcome != domain.OutcomeUnresolved || outcome.Reason != domain.ReasonSearchLimit || outcome.Evidence != domain.EvidenceNone || outcome.Location.Kind != domain.LocationUnresolved {
			t.Fatalf("unchecked later-thread search: %+v", outcome)
		}
	}
	after := read(contextReadRequest{})
	if *before.Thread.Tracking != *after.Thread.Tracking || !reflect.DeepEqual(before.Thread.Messages, after.Thread.Messages) {
		t.Fatal("exhaustion changed retained evidence/conversation")
	}
	if f.sourceRevision(contextReadRequest{}) != expectedSource {
		t.Fatal("sync changed source")
	}
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("unexpected cache/snapshot entries: %v", entries)
	}
}
