package cli_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const threadID = "018f0000-0000-7000-8000-000000000001"
const messageID = "018f0000-0000-7000-8000-000000000002"
const role = "---user <!-- sidecar-meta: {\"id\":\"" + messageID + "\"} -->\n"
const endThread = "[/@th_" + threadID + "]\n"
const canonicalNote = "[@th_" + threadID + " file]\n" + role + "x\n" + endThread

type subjectRequest struct{ T *testing.T }

func subjectBinary(r subjectRequest) string {
	r.T.Helper()
	binary := os.Getenv("SIDECAR_BIN")
	if binary == "" {
		r.T.Fatal("SIDECAR_BIN is required; helper/schema success is not Sidecar behavioral proof")
	}
	path, err := exec.LookPath(binary)
	if err != nil {
		r.T.Fatal(err)
	}
	path, err = filepath.Abs(path)
	if err != nil {
		r.T.Fatal(err)
	}
	return path
}

// BHV: absent and existing empty are distinct revision/warning states. Reads
// preserve both; check never cleans; plain format cleans only existing empty.
func TestSubjectEmptyLifecycle(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	emptyHash := fmt.Sprintf("sha256:%x", sha256.Sum256(nil))
	for _, exists := range []bool{false, true} {
		t.Run(fmt.Sprint(exists), func(t *testing.T) {
			dir := t.TempDir()
			initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}}
			storage, warnings := "null", "[]"
			if exists {
				initial = append(initial, fileRequest{Path: "note.txt.comments", Mode: 0640})
				storage = fmt.Sprintf("%q", emptyHash)
				warnings = `[{"code":"noncanonical_empty"}]`
			}
			for _, f := range initial {
				if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
					t.Fatal(err)
				}
			}
			for _, tc := range []struct {
				name         string
				args         []string
				status       int
				out, failure string
			}{
				{"validate", []string{"validate", "--file", "note.txt.comments"}, 0, fmt.Sprintf(`{"protocol":1,"command":"validate","changed":false,"result":{"valid":true,"warnings":%s,"storage_revision":%s,"canonical_revision":%q}}`+"\n", warnings, storage, emptyHash), ""},
				{"list", []string{"thread", "list", "--file", "note.txt.comments"}, 0, fmt.Sprintf(`{"protocol":1,"command":"thread-list","changed":false,"result":{"threads":[],"warnings":%s,"storage_revision":%s,"canonical_revision":%q}}`+"\n", warnings, storage, emptyHash), ""},
				{"get", []string{"thread", "get", "--file", "note.txt.comments", "--id", threadID}, 5, "", `{"protocol":1,"error":{"code":"not_found","message":"not_found:thread","details":{"entity":"thread","id":"` + threadID + `"}}}` + "\n"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					got, err := runProcess(runRequest{Binary: binary, Args: append([]string{"--output", "json"}, tc.args...), Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
					if err != nil {
						t.Fatal(err)
					}
					if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: tc.status, Stdout: []byte(tc.out), Stderr: []byte(tc.failure)}}); err != nil {
						t.Fatal(err)
					}
					if err := validateSchema(schemaRequest{JSON: []byte(tc.out + tc.failure)}); err != nil {
						t.Fatal(err)
					}
					files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
					if err != nil {
						t.Fatal(err)
					}
					if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
						t.Fatal(err)
					}
				})
			}
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "format", "--file", "note.txt.comments", "--check"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			status := 0
			if exists {
				status = 3
			}
			want := fmt.Sprintf(`{"protocol":1,"command":"format","changed":false,"result":{"formatted":false,"storage_revision":%s,"canonical_revision":%q}}`+"\n", storage, emptyHash)
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: status, Stdout: []byte(want)}}); err != nil {
				t.Fatal(err)
			}
			if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
				t.Fatal(err)
			}
			files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
				t.Fatal(err)
			}
			got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "format", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			want = fmt.Sprintf(`{"protocol":1,"command":"format","changed":%t,"result":{"formatted":%t,"storage_revision":null,"canonical_revision":%q}}`+"\n", exists, exists, emptyHash)
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
				t.Fatal(err)
			}
			if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
				t.Fatal(err)
			}
			files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files, Want: initial[:2]}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// BHV: finite formatter variants and canonical-first body LF preservation.
func TestSubjectFormatterVariants(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	twoRoles := "[@th_" + threadID + " file]\n" + role + "x\n\n---bot <!-- sidecar-meta: {\"id\":\"018f0000-0000-7000-8000-000000000003\"} -->\ny\n" + endThread
	second := strings.ReplaceAll(strings.ReplaceAll(canonicalNote, threadID, "018f0000-0000-7000-8000-000000000004"), messageID, "018f0000-0000-7000-8000-000000000005")
	second = strings.Replace(second, " file]", " lost]", 1)
	for _, tc := range []struct{ name, input, canonical string }{
		{"crlf", strings.ReplaceAll(canonicalNote, "\n", "\r\n"), canonicalNote},
		{"missing_final_lf", strings.TrimSuffix(canonicalNote, "\n"), canonicalNote},
		{"extra_final_lf", canonicalNote + "\n", canonicalNote},
		{"thread_order", second + "\n" + canonicalNote, canonicalNote + "\n" + second},
		{"thread_separator", canonicalNote + second, canonicalNote + "\n" + second},
		{"missing_role_separator", strings.Replace(twoRoles, "x\n\n---bot", "x\n---bot", 1), twoRoles},
		{"canonical_trailing_body_lf", strings.Replace(twoRoles, "x\n\n---bot", "x\n\n\n---bot", 1), strings.Replace(twoRoles, "x\n\n---bot", "x\n\n\n---bot", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(tc.input)}}
			for _, f := range initial {
				if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
					t.Fatal(err)
				}
			}
			storage := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(tc.input)))
			canonical := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(tc.canonical)))
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "format", "--file", "note.txt.comments", "--check"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			status := 0
			if tc.input != tc.canonical {
				status = 3
			}
			want := fmt.Sprintf(`{"protocol":1,"command":"format","changed":false,"result":{"formatted":false,"storage_revision":%q,"canonical_revision":%q}}`+"\n", storage, canonical)
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: status, Stdout: []byte(want)}}); err != nil {
				t.Fatal(err)
			}
			if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
				t.Fatal(err)
			}
			files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
				t.Fatal(err)
			}
			got, err = runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "format", "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			want = fmt.Sprintf(`{"protocol":1,"command":"format","changed":%t,"result":{"formatted":%t,"storage_revision":%q,"canonical_revision":%q}}`+"\n", tc.input != tc.canonical, tc.input != tc.canonical, canonical, canonical)
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Stdout: []byte(want)}}); err != nil {
				t.Fatal(err)
			}
			if err := validateSchema(schemaRequest{JSON: got.Stdout}); err != nil {
				t.Fatal(err)
			}
			initial[2].Bytes = []byte(tc.canonical)
			files, err = observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// BHV: public-byte rejection campaign. These exact encoding/ending/location
// symbols are specified; no guessed generic usage or domain mapping is used.
func TestSubjectMalformedBytes(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, tc := range []struct{ name, input, path, reason string }{
		{"bom", "\xef\xbb\xbf" + canonicalNote, "target_encoding", "bom"},
		{"nul", strings.Replace(canonicalNote, "x\n", "x\x00\n", 1), "target_encoding", "nul"},
		{"utf8", strings.Replace(canonicalNote, "x\n", "\xff\n", 1), "target_encoding", "utf8"},
		{"bare_cr", strings.Replace(canonicalNote, "x\n", "x\r", 1), "target_endings", "endings"},
		{"mixed", strings.Replace(canonicalNote, "x\n", "x\r\n", 1), "target_endings", "endings"},
		{"zero_line", strings.Replace(canonicalNote, " file]", " line:0]", 1), "target_content", "location"},
		{"reversed_range", strings.Replace(canonicalNote, " file]", " range:2:1-1:1]", 1), "target_content", "location"},
		{"empty_anchor", strings.Replace(canonicalNote, " file]", " range:1:1-1:2 | ]", 1), "target_content", "location"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			initial := []fileRequest{{Path: ".note.txt.comments.sidecar.lock", Mode: 0600}, {Path: "note.txt", Mode: 0640, Bytes: []byte("source\n")}, {Path: "note.txt.comments", Mode: 0640, Bytes: []byte(tc.input)}}
			for _, f := range initial {
				if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
					t.Fatal(err)
				}
			}
			for _, command := range []string{"validate", "format"} {
				got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", command, "--file", "note.txt.comments"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
				if err != nil {
					t.Fatal(err)
				}
				want := fmt.Sprintf(`{"protocol":1,"error":{"code":"validation","message":"validation:%s:%s","details":{"path":%q,"reason":%q}}}`+"\n", tc.path, tc.reason, tc.path, tc.reason)
				if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 3, Stderr: []byte(want)}}); err != nil {
					t.Fatal(err)
				}
				if err := validateSchema(schemaRequest{JSON: got.Stderr}); err != nil {
					t.Fatal(err)
				}
				files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
				if err != nil {
					t.Fatal(err)
				}
				if err := compareFiles(filesExpectation{Got: files, Want: initial}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
