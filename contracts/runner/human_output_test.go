package runner

import "testing"

func TestHumanPresentationIsNotMachineProtocol(t *testing.T) {
	for _, argv := range [][]string{{"version"}, {"--output", "text", "version"}} {
		fixture := golden{Argv: argv}
		// Deliberately not JSON, nor the old one-line result transport.
		if err := validateEnvelopes(fixture, 0, []byte("Sidecar test\nFormat version: 1\nProtocol version: 1\n"), nil); err != nil {
			t.Fatalf("human version rejected: %v", err)
		}
	}
	failure := []byte("Cannot read the sidecar.\nCheck the path and permissions.\n")
	if err := validateEnvelopes(golden{Argv: []string{"format"}}, 7, nil, failure); err != nil {
		t.Fatalf("human diagnostic rejected: %v", err)
	}
	for _, output := range []string{"json", "id", "none"} {
		fixture := golden{Argv: []string{"--output", output, "format"}}
		if err := validateEnvelopes(fixture, 7, nil, failure); err == nil {
			t.Fatalf("machine mode %s accepted prose", output)
		}
		fixture = golden{Argv: []string{"--output", output, "wat"}, Expect: expected{InvocationError: "argv"}}
		if err := validateEnvelopes(fixture, 2, nil, failure); err == nil {
			t.Fatalf("machine argv error mode %s accepted prose", output)
		}
	}
}

func TestHelpIsHumanEvenWithJSONSelected(t *testing.T) {
	for _, argv := range [][]string{
		{}, {"--help"}, {"-h"}, {"thread"}, {"message"},
		{"thread", "--help"}, {"message", "add", "-h"},
		{"--output", "json", "version", "--help"},
		{"--version", "--help"}, {"--version", "-h"},
		{"--output", "json", "--version", "--help"},
		{"--output", "json", "--version", "-h"},
		{"--output=json", "--version", "--help"},
		{"--output=json", "format", "--lock-timeout", "not-a-duration", "--help"},
		{"--output=json", "format", "--check=not-a-bool", "--help"},
		{"--output", "id", "thread", "--help"},
		{"--output", "yaml", "--help"},
	} {
		invocation, err := parseInvocation(argv)
		if err != nil || invocation.command != "help" || jsonOutput(argv) {
			t.Fatalf("help invocation %q: %+v, %v", argv, invocation, err)
		}
		if err := validateEnvelopes(golden{Argv: argv}, 0, []byte("Usage: sidecar\n\nCommands and options\n"), nil); err != nil {
			t.Fatalf("human help rejected for %q: %v", argv, err)
		}
	}
}

func TestEarlierInvocationFailuresCannotSelectHumanHelp(t *testing.T) {
	for _, args := range [][]string{
		{"version", "--unknown", "--help"},
		{"version", "--file", "x", "--help"},
		{"version", "unexpected", "--help"},
		{"wat", "--help"},
		{"thread", "wat", "--help"},
		{"validate", "--file", "a.comments", "--file", "b.comments", "--help"},
		{"format", "--file", "a.comments", "--if-revision", "missing", "--help"},
		{"format", "--check", "--lock-timeout", "1s", "--help"},
		{"version", "--if-revision", "missing", "--help"},
		{"--output", "json", "version", "--help"},
	} {
		for _, selector := range [][]string{{"--output", "json"}, {"--output=json"}} {
			argv := append(append([]string{}, selector...), args...)
			if !jsonOutput(argv) {
				t.Fatalf("earlier failure selected human help: %q", argv)
			}
			fixture := golden{Argv: argv}
			if _, err := parseInvocation(argv); err != nil {
				fixture.Expect.InvocationError = "argv"
			}
			usage := []byte("{\"protocol\":1,\"error\":{\"code\":\"usage\",\"message\":\"usage:argv:malformed\",\"details\":{\"field\":\"argv\",\"reason\":\"malformed\"}}}\n")
			if err := validateEnvelopes(fixture, 2, nil, usage); err != nil {
				t.Fatalf("JSON usage rejected for %q: %v", argv, err)
			}
			if err := validateEnvelopes(fixture, 2, nil, []byte("Invalid invocation.\n")); err == nil {
				t.Fatalf("JSON failure accepted prose: %q", argv)
			}
		}
	}
	for _, argv := range [][]string{
		{"--output", "id", "version", "--help"},
		{"--output", "none", "validate", "--help"},
		{"--output", "yaml", "version", "--help"},
	} {
		invocation, _ := parseInvocation(argv)
		if invocation.command == "help" {
			t.Fatalf("invalid output selection reached command help: %q", argv)
		}
	}
}

func TestHelpTokenAsFlagValueRetainsMachineValidation(t *testing.T) {
	for _, args := range [][]string{
		{"--author-name", "--help"}, {"--author-name", "-h"}, {"--author-name=--help"},
	} {
		argv := append([]string{"--output", "json", "thread", "create"}, args...)
		if !jsonOutput(argv) {
			t.Fatalf("flag value incorrectly disabled JSON schema validation: %q", argv)
		}
	}
}

func TestExplicitMachineMissingCommandStillRequiresUsageEnvelope(t *testing.T) {
	fixture := golden{Argv: []string{"--output", "json"}, Expect: expected{InvocationError: "argv"}}
	valid := []byte("{\"protocol\":1,\"error\":{\"code\":\"usage\",\"message\":\"usage:argv:missing\",\"details\":{\"field\":\"argv\",\"reason\":\"missing\"}}}\n")
	if err := validateEnvelopes(fixture, 2, nil, valid); err != nil {
		t.Fatal(err)
	}
	if err := validateEnvelopes(fixture, 2, nil, []byte("Usage: sidecar\n")); err == nil {
		t.Fatal("JSON missing command accepted human help instead of error envelope")
	}
}
