package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

func TestSubjectContextCreateRetryDoesNotRecapture(t *testing.T) {
	for _, mode := range []string{"ordinary", "apply"} {
		t.Run(mode, func(t *testing.T) {
			f := newContextFixture(contextFixtureRequest{T: t, Source: "prefix chosen suffix\n", Location: "range:1:8-1:14"})
			original := f.read(contextReadRequest{})
			location, err := json.Marshal(original.Thread.Location)
			if err != nil {
				t.Fatal(err)
			}
			author, err := json.Marshal(original.Thread.Messages[0].Author)
			if err != nil {
				t.Fatal(err)
			}
			f.source(contextSourceRequest{Text: "prefix altered suffix\n"})
			for _, step := range []struct {
				name                    string
				guard, remove, mismatch bool
				status                  int
			}{
				{name: "edited_source"},
				{name: "supplied_guard_still_checked", guard: true, status: domain.ExitConflict},
				{name: "missing_source", remove: true},
				{name: "mismatch_without_source", mismatch: true, status: domain.ExitPrecondition},
			} {
				t.Run(step.name, func(t *testing.T) {
					f := f
					f.t = t
					if step.remove {
						if err := os.Remove(filepath.Join(f.dir, contextSource)); err != nil {
							t.Fatal(err)
						}
					}
					body := original.Thread.Messages[0].Body
					if step.mismatch {
						body += " Different retry."
					}
					request := contextCallRequest{ExpectedStatus: step.status, Error: step.status != 0}
					if mode == "ordinary" {
						request.Args = []string{"thread", "create"}
						if step.guard {
							request.Args = append(request.Args, "--if-source-revision", original.Thread.Tracking.SourceRevision)
						}
						request.Args = append(request.Args, "--file", contextTarget, "--location", domain.LocationText(domain.LocationTextRequest{Location: original.Thread.Location}), "--author-kind", original.Thread.Messages[0].Author.Kind, "--body-file", "-", "--id", original.Thread.ID, "--message-id", original.Thread.Messages[0].ID)
						request.Stdin = []byte(body)
					} else {
						encodedBody, err := json.Marshal(body)
						if err != nil {
							t.Fatal(err)
						}
						guard := ""
						if step.guard {
							guard = fmt.Sprintf(`"if_source_revision":%q,`, original.Thread.Tracking.SourceRevision)
						}
						request.Args = []string{"apply", "--file", contextTarget, "--operations-file", "-"}
						request.Stdin = []byte(fmt.Sprintf(`{"protocol":%d,%s"operations":[{"op":"thread_create","thread_id":%q,"message_id":%q,"location":%s,"author":%s,"body":%s}]}`, domain.ProtocolVersion, guard, original.Thread.ID, original.Thread.Messages[0].ID, location, author, encodedBody))
						if err := validateSchema(schemaRequest{JSON: request.Stdin, Path: contextSchemaPath}); err != nil {
							t.Fatal(err)
						}
					}
					before := f.snapshot(contextReadRequest{})
					result := f.observe(request)
					if step.status == 0 {
						var envelope contextEnvelope
						if err := json.Unmarshal(result.Stdout, &envelope); err != nil || envelope.Changed {
							t.Fatalf("retry changed state: %s; error=%v", result.Stdout, err)
						}
					} else if step.mismatch && !bytes.Contains(result.Stderr, []byte(`"reason":"retry_mismatch"`)) {
						t.Fatalf("retry mismatch was masked by source access: %s", result.Stderr)
					}
					if err := compareFiles(filesExpectation{Got: f.snapshot(contextReadRequest{}), Want: before}); err != nil {
						t.Fatal(err)
					}
					if got := f.read(contextReadRequest{}); *got.Thread.Tracking != *original.Thread.Tracking || got.StorageRevision != original.StorageRevision {
						t.Fatal("retry recaptured or rewrote tracking evidence")
					}
				})
			}
		})
	}
}
