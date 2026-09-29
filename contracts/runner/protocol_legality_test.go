package runner

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExitThreeSuccessIsLimitedToChecks(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		ok   bool
	}{
		{name: "validate", argv: []string{"validate"}, ok: true},
		{name: "format check", argv: []string{"format", "--check"}, ok: true},
		{name: "fix check", argv: []string{"fix", "--check"}, ok: true},
		{name: "format without check", argv: []string{"format"}},
		{name: "apply", argv: []string{"apply"}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			g := golden{Argv: test.argv}
			err := validateEnvelopes(g, 3, []byte(parseCommand(test.argv)+" changed=false result={}\n"), nil)
			if (err == nil) != test.ok {
				t.Fatalf("validateEnvelopes() error = %v, want success %t", err, test.ok)
			}
		})
	}
}

func TestIDOutputRequiresLFAndSuccessfulExit(t *testing.T) {
	g := golden{Argv: []string{"--output", "id", "thread", "create"}}
	id := []byte("018f47a0-1234-7abc-8def-123456789abc\n")
	if err := validateEnvelopes(g, 0, id, nil); err != nil {
		t.Fatalf("valid LF-terminated ID output rejected: %v", err)
	}
	for _, output := range [][]byte{
		[]byte("018f47a0-1234-7abc-8def-123456789abc"),
		[]byte("018f47a0-1234-7abc-8def-123456789abc\n\n"),
	} {
		if err := validateEnvelopes(g, 0, output, nil); err == nil {
			t.Fatalf("malformed ID output %q was accepted", output)
		}
	}
	if err := validateEnvelopes(g, 3, id, nil); err == nil {
		t.Fatal("ID output was accepted with a nonzero exit status")
	}
}

func TestTextErrorCodeMustMatchExitStatus(t *testing.T) {
	g := golden{Argv: []string{"apply"}}
	if err := validateEnvelopes(g, 3, nil, []byte("error io io:read\n")); err == nil {
		t.Fatal("io error was accepted with validation exit status")
	}
	if err := validateEnvelopes(g, 7, nil, []byte("error io io:read\n")); err != nil {
		t.Fatalf("io error with its required exit status rejected: %v", err)
	}
}

func TestAssertFSRestoresVerifiedZeroModeAfterReading(t *testing.T) {
	root := t.TempDir()
	want := fsSpec{Entries: []entry{{
		Path:  "locked",
		Kind:  "file",
		Bytes: "c2VjcmV0",
		Mode:  "0000",
	}}}
	if err := materialize(materializeRequest{Root: root, FS: want}); err != nil {
		t.Fatal(err)
	}
	if err := assertFS(assertFSRequest{Root: root, Want: want}); err != nil {
		t.Fatalf("could not assert unchanged 0000 file: %v", err)
	}
	info, err := os.Lstat(filepath.Join(root, "locked"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0000 {
		t.Fatalf("post-run assertion changed mode: got %04o want 0000", info.Mode().Perm())
	}
}

func parseCommand(argv []string) string {
	invocation, err := parseInvocation(argv)
	if err != nil {
		panic(err)
	}
	return invocation.command
}
