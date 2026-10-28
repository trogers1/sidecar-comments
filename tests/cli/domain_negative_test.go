package cli_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These are invalid stored documents, not request usage failures. The closed
// validation vocabulary assigns content/domain invalidity to target_content.
func TestSubjectInvalidStoredDomain(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	metadata := `{"id":"` + messageID + `"}`
	for _, tc := range []struct{ name, meta, body string }{
		{"duplicate_id", `{"id":"` + messageID + `","id":"` + messageID + `"}`, "x"},
		{"unknown", `{"id":"` + messageID + `","extra":1}`, "x"},
		{"array", `[]`, "x"}, {"null", `null`, "x"}, {"numeric_id", `{"id":1}`, "x"},
		{"uuid_uppercase", `{"id":"018F0000-0000-7000-8000-000000000002"}`, "x"},
		{"uuid_version", `{"id":"018f0000-0000-4000-8000-000000000002"}`, "x"},
		{"uuid_variant", `{"id":"018f0000-0000-7000-0000-000000000002"}`, "x"},
		{"uuid_nil", `{"id":"00000000-0000-0000-0000-000000000000"}`, "x"},
		{"surrogate", `{"id":"` + messageID + `","name":"\ud800"}`, "x"},
		{"decomposed_name", `{"id":"` + messageID + `","name":"é"}`, "x"},
		{"empty_name", `{"id":"` + messageID + `","name":""}`, "x"},
		{"name_control", `{"id":"` + messageID + `","name":"\u0001"}`, "x"},
		{"name_129", `{"id":"` + messageID + `","name":"` + strings.Repeat("é", 129) + `"}`, "x"},
		{"gitlab_fraction", `{"id":"` + messageID + `","gitlab":{"kind":"system_note","project_id":1.0,"note_id":1}}`, "x"},
		{"gitlab_exponent", `{"id":"` + messageID + `","gitlab":{"kind":"system_note","project_id":1e0,"note_id":1}}`, "x"},
		{"gitlab_overflow", `{"id":"` + messageID + `","gitlab":{"kind":"system_note","project_id":9223372036854775808,"note_id":1}}`, "x"},
		{"gitlab_zero", `{"id":"` + messageID + `","gitlab":{"kind":"system_note","project_id":0,"note_id":1}}`, "x"},
		{"gitlab_nested_duplicate", `{"id":"` + messageID + `","gitlab":{"kind":"system_note","project_id":1,"note_id":1,"note_id":2}}`, "x"},
		{"gitlab_unknown", `{"id":"` + messageID + `","gitlab":{"kind":"system_note","project_id":1,"note_id":1,"extra":1}}`, "x"},
		{"human_external_author", `{"id":"` + messageID + `","external_author":{"id":"x","name":"X"}}`, "x"},
		{"body_65537", metadata, strings.Repeat("x", 65537)},
		{"reserved_user", metadata, "---userish"}, {"reserved_bot", metadata, "---botany"}, {"reserved_external", metadata, "---externality"},
		{"reserved_start", metadata, "[@th_invalid"}, {"reserved_end", metadata, "[/@th_invalid"},
		{"whitespace", metadata, " \t\u0085\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			document := []byte("[@th_" + threadID + " lost]\n---user <!-- sidecar-meta: " + tc.meta + " -->\n" + tc.body + "\n" + endThread)
			initial := []fileRequest{{Path: "note.txt.comments", Mode: 0640, Bytes: document}}
			if err := writeFile(fileRequest{Path: filepath.Join(dir, initial[0].Path), Mode: initial[0].Mode, Bytes: document}); err != nil {
				t.Fatal(err)
			}
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "validate", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			want := `{"protocol":1,"error":{"code":"validation","message":"validation:target_content:invalid","details":{"path":"target_content","reason":"invalid"}}}` + "\n"
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 3, Stderr: []byte(want)}}); err != nil {
				t.Fatal(err)
			}
			if err := validateSchema(schemaRequest{JSON: got.Stderr}); err != nil {
				t.Fatal(err)
			}
			files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 1 << 20})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
