package cli_test

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubjectAnchorBoundary(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, size := range []int{4096, 4097} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			dir := t.TempDir()
			source := strings.Repeat("x", size)
			anchor := base64.StdEncoding.EncodeToString([]byte(source))
			document := []byte(strings.Replace(canonicalNote, " file]", fmt.Sprintf(" range:1:1-1:%d | %s]", size+1, anchor), 1))
			initial := []fileRequest{{Path: "note.txt", Mode: 0640, Bytes: []byte(source)}, {Path: "note.txt.comments", Mode: 0640, Bytes: document}}
			for _, f := range initial {
				if err := writeFile(fileRequest{Path: filepath.Join(dir, f.Path), Mode: f.Mode, Bytes: f.Bytes}); err != nil {
					t.Fatal(err)
				}
			}
			got, err := runProcess(runRequest{Binary: binary, Args: []string{"--output", "json", "validate", "--file", "note.txt.comments", "--source", "note.txt"}, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			revision := fmt.Sprintf("sha256:%x", sha256.Sum256(document))
			want := processResult{Stdout: []byte(fmt.Sprintf(`{"protocol":1,"command":"validate","changed":false,"result":{"valid":true,"warnings":[],"storage_revision":%q,"canonical_revision":%q}}`+"\n", revision, revision))}
			if size > 4096 {
				want = processResult{Status: 3, Stderr: []byte(`{"protocol":1,"error":{"code":"validation","message":"validation:target_content:location","details":{"path":"target_content","reason":"location"}}}` + "\n")}
			}
			if err := compareProcess(processExpectation{Got: got, Want: want}); err != nil {
				t.Fatal(err)
			}
			if err := validateSchema(schemaRequest{JSON: append(append([]byte{}, got.Stdout...), got.Stderr...)}); err != nil {
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
