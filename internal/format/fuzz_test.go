package format_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/contracts/runner"
	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/format"
)

// Exercise the production parser directly: its grammar is complex, and the CLI's
// blanket panic recovery would hide exactly the failures these properties seek.
// Inputs are bounded by the production limits; go test's timeout guards hangs.
type corpusRequest struct{ T testing.TB }

// Reuse the checked-in golden and its closed runner DTO/schema rather than
// inventing another canonical ID spelling. Do this once per target setup.
func corpusDocument(r corpusRequest) domain.DocumentValue {
	r.T.Helper()
	if err := runner.ValidateArtifacts("../../contracts/corpus/v1.json"); err != nil {
		r.T.Fatal(err)
	}
	data, err := os.ReadFile("../../contracts/goldens/v1/message-add.json")
	if err != nil {
		r.T.Fatal(err)
	}
	var golden runner.Golden
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&golden); err != nil {
		r.T.Fatal(err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		r.T.Fatalf("golden trailing JSON: %v", err)
	}
	for _, entry := range golden.Expect.Final.Entries {
		if entry.Path != "a.txt.comments" {
			continue
		}
		if entry.Kind != "file" {
			r.T.Fatal("canonical fixture target is not a file")
		}
		content, err := base64.StdEncoding.Strict().DecodeString(entry.Bytes)
		if err != nil {
			r.T.Fatal(err)
		}
		parsed, err := format.Parse(format.ParseRequest{Bytes: content, Mode: "normal"})
		if err != nil {
			r.T.Fatal(err)
		}
		if parsed.Repair != nil {
			r.T.Fatal("canonical fixture needs repair")
		}
		threads := parsed.Value.Threads(domain.DocumentThreadsRequest{})
		if len(threads) != 1 || len(threads[0].Messages) != 2 {
			r.T.Fatal("message-add golden must supply one thread and two messages")
		}
		return parsed.Value
	}
	r.T.Fatal("canonical target absent from message-add golden")
	return domain.DocumentValue{}
}

type fixtureRequest struct {
	T      testing.TB
	Body   string
	Corpus domain.DocumentValue
}

func fixture(r fixtureRequest) domain.DocumentValue {
	r.T.Helper()
	source := r.Corpus.Threads(domain.DocumentThreadsRequest{})[0]
	// Thread and message IDs have distinct namespaces, so the validated thread
	// ID also supplies a third distinct message ID without synthesizing a UUID.
	ids := []string{source.Messages[0].ID, source.Messages[1].ID, source.ID}
	messages := []domain.Message{}
	for i, kind := range []string{domain.AuthorHuman, domain.AuthorAgent, domain.AuthorExternal} {
		id := ids[i]
		name := "Zoë \"<&>\\"
		author := domain.Author{Kind: kind, Name: &name}
		if kind == domain.AuthorExternal {
			author.Name = nil
			author.ExternalAuthor = &domain.ExternalAuthor{ID: "external", Name: "Zoë"}
		}
		discussion := "discussion"
		messages = append(messages, domain.Message{ID: id, Author: author, Body: r.Body, GitLab: &domain.GitLab{Kind: domain.GitLabNote, ProjectID: 1, NoteID: 2, DiscussionID: &discussion}})
	}
	value, err := domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: []domain.Thread{{ID: source.ID, Location: domain.Location{Kind: domain.LocationFile}, Messages: messages}}})
	if err != nil {
		r.T.Fatal(err)
	}
	return value
}

type roundtripRequest struct {
	T     testing.TB
	Value domain.DocumentValue
}

func checkRoundtrip(r roundtripRequest) {
	r.T.Helper()
	encoded := format.Encode(format.EncodeRequest{Document: r.Value})
	parsed, err := format.Parse(format.ParseRequest{Bytes: encoded, Mode: "normal"})
	if err != nil {
		r.T.Fatalf("encoded document rejected: %v; bytes=%q", err, encoded)
	}
	if parsed.Repair != nil {
		r.T.Fatal("canonical document needs repair")
	}
	// Thread order is canonicalized by encoding, but every DTO field (including
	// exact body bytes, trailing LF and blank lines) must survive unchanged.
	want := r.Value.Threads(domain.DocumentThreadsRequest{})
	got := parsed.Value.Threads(domain.DocumentThreadsRequest{})
	byID := map[string]domain.Thread{}
	for _, thread := range got {
		byID[thread.ID] = thread
	}
	if len(want) != len(got) {
		r.T.Fatal("thread count changed")
	}
	for _, thread := range want {
		if !reflect.DeepEqual(thread, byID[thread.ID]) {
			r.T.Fatalf("semantic change: want %#v, got %#v", thread, byID[thread.ID])
		}
	}
	again := format.Encode(format.EncodeRequest{Document: parsed.Value})
	if !bytes.Equal(encoded, again) {
		r.T.Fatal("canonical encoding not idempotent")
	}
	revision := domain.HashRevision(domain.HashRevisionRequest{Bytes: encoded})
	if revision != domain.HashRevision(domain.HashRevisionRequest{Bytes: again}) || domain.ValidateRevision(domain.ValidateRevisionRequest{Text: revision}) != nil {
		r.T.Fatal("unstable or invalid revision")
	}
}

func FuzzParseDocument(f *testing.F) {
	corpus := corpusDocument(corpusRequest{T: f})
	canonical := format.Encode(format.EncodeRequest{Document: fixture(fixtureRequest{T: f, Body: "first\n\nlast\n\n", Corpus: corpus})})
	formatter := bytes.ReplaceAll(canonical, []byte("\n"), []byte("\r\n"))
	metadataSpacing := bytes.ReplaceAll(canonical, []byte(`{"id":`), []byte(`{ "id": `))
	// Derive the missing-metadata role line from the encoder, not a second
	// definition of the metadata schema or ID grammar.
	lines := strings.Split(string(canonical), "\n")
	lines[1] = "---" + domain.RoleUser
	repair := []byte(strings.Join(lines, "\n"))
	for _, seed := range [][]byte{nil, canonical, formatter, metadataSpacing, bytes.TrimSuffix(canonical, []byte("\n")), repair, []byte("\xff"), append([]byte{0}, canonical...), append([]byte("\xef\xbb\xbf"), canonical...), canonical[:len(canonical)/2]} {
		f.Add(seed)
	}
	// Pin the corpus categories so accidental seed drift cannot remove coverage.
	for _, seed := range []struct {
		data   []byte
		mode   string
		repair bool
	}{{canonical, "normal", false}, {formatter, "format", false}, {metadataSpacing, "format", false}, {repair, "repair", true}} {
		parsed, err := format.Parse(format.ParseRequest{Bytes: seed.data, Mode: seed.mode})
		if err != nil || (parsed.Repair != nil) != seed.repair {
			f.Fatalf("seed %s: %v", seed.mode, err)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > domain.MaxTargetBytes+1 {
			t.Skip()
		}
		for _, mode := range []string{"normal", "format", "repair", "invalid"} {
			parsed, err := format.Parse(format.ParseRequest{Bytes: data, Mode: mode})
			if err != nil {
				continue
			}
			if mode == "invalid" {
				t.Fatal("accepted invalid mode")
			}
			if parsed.Repair != nil {
				if mode != "repair" || len(parsed.Repair.Diagnostics) == 0 {
					t.Fatal("unexpected repair result")
				}
				for _, d := range parsed.Repair.Diagnostics {
					if d.ByteOffset < 1 || d.ByteOffset > len(data) || d.Line < 1 || d.MessageOrdinal < 1 {
						t.Fatalf("invalid diagnostic: %+v", d)
					}
				}
				continue
			}
			if mode == "normal" && !bytes.Equal(data, format.Encode(format.EncodeRequest{Document: parsed.Value})) {
				t.Fatal("normal parse accepted noncanonical bytes")
			}
			checkRoundtrip(roundtripRequest{T: t, Value: parsed.Value})
		}
	})
}

func FuzzDocumentBodyRoundTrip(f *testing.F) {
	corpus := corpusDocument(corpusRequest{T: f})
	for _, body := range []string{"text", "text\n", "text\n\n", "\n\ntext\n\n", "a\n\nb", "é\t<&>"} {
		f.Add(body)
	}
	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > domain.MaxBodyBytes+1 {
			t.Skip()
		}
		// Validate the untrusted fixture with the production schema before building
		// a value; rejection is expected for arbitrary reserved lines/invalid UTF-8.
		if domain.ValidateBody(domain.ValidateBodyRequest{Body: body}) != nil {
			return
		}
		checkRoundtrip(roundtripRequest{T: t, Value: fixture(fixtureRequest{T: t, Body: body, Corpus: corpus})})
	})
}
