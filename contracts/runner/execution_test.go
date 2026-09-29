package runner

import (
	"bytes"
	"encoding/base64"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestRunnerExecutionHelperProcess is an executable fixture used by the
// bounded-execution tests below. It is never a test itself in the parent.
func TestRunnerExecutionHelperProcess(t *testing.T) {
	if os.Getenv("SIDECAR_RUNNER_HELPER") != "1" {
		return
	}
	switch os.Getenv("SIDECAR_RUNNER_HELPER_MODE") {
	case "flood":
		block := bytes.Repeat([]byte("x"), 64<<10)
		for {
			_, _ = os.Stdout.Write(block)
		}
	case "descendant":
		time.Sleep(time.Second)
	case "hold-descendant":
		child := exec.Command(os.Args[0], "-test.run=^TestRunnerExecutionHelperProcess$")
		child.Env = append(os.Environ(), "SIDECAR_RUNNER_HELPER=1", "SIDECAR_RUNNER_HELPER_MODE=descendant")
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		time.Sleep(time.Second)
	}
	os.Exit(0)
}

func helperCommand(mode string) *exec.Cmd {
	command := exec.Command(os.Args[0], "-test.run=^TestRunnerExecutionHelperProcess$")
	command.Env = append(os.Environ(), "SIDECAR_RUNNER_HELPER=1", "SIDECAR_RUNNER_HELPER_MODE="+mode)
	return command
}

func TestLiteralBase64ExpectationIsNeverTemplateExpanded(t *testing.T) {
	literal := []byte("{{CLI_VERSION}} {{UUIDV7:bind:id}}")
	encoded := base64.StdEncoding.EncodeToString(literal)
	got, err := expectedBytes(expectedBytesRequest{Encoded: &encoded, Expansion: expandRequest{CLIVersion: "changed"}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, literal) {
		t.Fatalf("literal base64 was interpreted: got %q want %q", got, literal)
	}
	if err := matchExpectedStream(literal, literal, false, newUUIDBindings(), fsSpec{}, fsSpec{}); err != nil {
		t.Fatalf("literal stream was interpreted as a template: %v", err)
	}
	if err := matchExpectedStream(literal, []byte("changed"), false, newUUIDBindings(), fsSpec{}, fsSpec{}); err == nil {
		t.Fatal("different literal bytes matched")
	}
	input := golden{Stdin: encoded, Initial: fsSpec{Entries: []entry{{Bytes: encoded}}}}
	if err := rejectUUIDTemplatesInInputs(input); err != nil {
		t.Fatalf("literal fixture input was interpreted as a template: %v", err)
	}

	template := "{{CLI_VERSION}}"
	got, err = expectedBytes(expectedBytesRequest{Template: &template, Expansion: expandRequest{CLIVersion: "changed"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "changed" {
		t.Fatalf("template field was not expanded: got %q", got)
	}
}

func TestBoundedCaptureRejectsOutputFlood(t *testing.T) {
	capture := boundedBuffer{limit: 1024}
	if _, err := capture.Write(bytes.Repeat([]byte("x"), 64<<10)); err == nil {
		t.Fatal("unbounded output was accepted")
	}
	if !capture.overflow {
		t.Fatal("unbounded output was not marked as rejected")
	}
	if capture.Len() > capture.limit {
		t.Fatalf("capture grew beyond limit: got %d want at most %d", capture.Len(), capture.limit)
	}
}

func TestTimeoutDoesNotWaitForDescendantHeldPipe(t *testing.T) {
	command := helperCommand("hold-descendant")
	var output bytes.Buffer
	command.Stdout = &output
	started := time.Now()
	err := runCommand(commandRequest{Command: command, Timeout: 25 * time.Millisecond})
	if err == nil {
		t.Fatal("timed-out process succeeded")
	}
	if elapsed := time.Since(started); elapsed > 750*time.Millisecond {
		t.Fatalf("timeout waited for descendant-held pipe: %s", elapsed)
	}
}

func TestPermissionCapabilityReportsActualAvailability(t *testing.T) {
	probe := checkPermission()
	capability := requireCapabilities([]string{"permission"})
	if probe == nil && capability != nil {
		t.Fatalf("permission probe succeeded but capability is unavailable: %v", capability)
	}
	if probe != nil && capability == nil {
		t.Fatal("permission probe failed but capability was reported available")
	}
	// Privileged runners may read 0000 files. That is an unavailable fixture
	// capability, not a failing harness test or permission-denial proof.
}
