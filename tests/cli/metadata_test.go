package cli_test

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Stored valid-domain boundaries are observed through get, not an internal
// constructor or serializer. Metadata bytes and public DTOs are independent.
func TestSubjectStoredMetadata(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, tc := range []struct{ name, role, metadata, author, body, gitlab, id string }{
		{"nfc_nonbmp", "user", `{"id":"` + messageID + `","name":"É😀"}`, `{"kind":"human","name":"É😀"}`, "😀", "", messageID},
		{"name_128_scalars", "user", `{"id":"` + messageID + `","name":"` + strings.Repeat("😀", 128) + `"}`, `{"kind":"human","name":"` + strings.Repeat("😀", 128) + `"}`, "x", "", messageID},
		{"external_256_scalars", "external", `{"id":"` + messageID + `","external_author":{"id":"` + strings.Repeat("😀", 256) + `","name":"X"}}`, `{"kind":"external","external_author":{"id":"` + strings.Repeat("😀", 256) + `","name":"X"}}`, "x", "", messageID},
		{"agent", "bot", `{"id":"` + messageID + `"}`, `{"kind":"agent"}`, "x", "", messageID},
		{"external_decomposed_identity", "external", `{"id":"` + messageID + `","external_author":{"id":"é","name":"É"}}`, `{"kind":"external","external_author":{"id":"é","name":"É"}}`, "x", "", messageID},
		{"gitlab_note_max_integer", "user", `{"id":"` + messageID + `","gitlab":{"kind":"note","project_id":9223372036854775807,"discussion_id":"discussion","note_id":1}}`, `{"kind":"human"}`, "x", `,"gitlab":{"kind":"note","project_id":9223372036854775807,"discussion_id":"discussion","note_id":1}`, messageID},
		{"gitlab_system_note", "bot", `{"id":"` + messageID + `","gitlab":{"kind":"system_note","project_id":1,"note_id":9223372036854775807}}`, `{"kind":"agent"}`, "x", `,"gitlab":{"kind":"system_note","project_id":1,"note_id":9223372036854775807}`, messageID},
		{"distinct_id_namespaces", "user", `{"id":"` + threadID + `"}`, `{"kind":"human"}`, "x", "", threadID},
		{"not_unicode_whitespace", "user", `{"id":"` + messageID + `"}`, `{"kind":"human"}`, "\u200b", "", messageID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			document := []byte("[@th_" + threadID + " lost]\n---" + tc.role + " <!-- sidecar-meta: " + tc.metadata + " -->\n" + tc.body + "\n" + endThread)
			initial := []fileRequest{{Path: "note.txt.comments", Mode: 0640, Bytes: document}}
			if err := writeFile(fileRequest{Path: filepath.Join(dir, initial[0].Path), Mode: initial[0].Mode, Bytes: document}); err != nil {
				t.Fatal(err)
			}
			revision := fmt.Sprintf("sha256:%x", sha256.Sum256(document))
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "thread", "get", "--file", "note.txt.comments", "--id", threadID}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 8192})
			if err != nil {
				t.Fatal(err)
			}
			// Raw UTF-8 canonical JSON never applies HTML escaping or NFC rewriting.
			want := fmt.Sprintf(`{"protocol":1,"command":"thread-get","changed":false,"result":{"thread":{"id":%q,"location":{"kind":"lost"},"messages":[{"id":%q,"author":%s,"body":"%s"%s}]},"storage_revision":%q,"canonical_revision":%q}}`+"\n", threadID, tc.id, tc.author, tc.body, tc.gitlab, revision, revision)
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
				t.Fatal(err)
			}
			if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
				t.Fatal(err)
			}
			files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 8192})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
