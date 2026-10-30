package cli_test

import (
	"testing"
	"time"
)

func TestSubjectVersionAlias(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, mode := range []string{"text", "json"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			request := runRequest{Binary: binary, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096}
			request.Args = []string{"--output", mode, "version"}
			command, err := runProcess(request)
			if err != nil {
				t.Fatal(err)
			}
			if command.Status != 0 || len(command.Stderr) != 0 {
				t.Fatalf("version failed: %+v", command)
			}
			request.Args = []string{"--output", mode, "--version"}
			alias, err := runProcess(request)
			if err != nil {
				t.Fatal(err)
			}
			if err := compareProcess(processExpectation{Got: alias, Want: command}); err != nil {
				t.Fatal(err)
			}
			if mode == "json" {
				if err := validateSchema(schemaRequest{JSON: alias.Stdout}); err != nil {
					t.Fatal(err)
				}
			} else {
				assertHumanOutput(humanOutputExpectation{T: t, Got: alias, Terms: []string{"Sidecar", "format", "protocol"}})
			}
			files, err := observeFiles(filesRequest{Dir: dir, ByteLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareFiles(filesExpectation{Got: files}); err != nil {
				t.Fatal(err)
			}
		})
	}
	// The unqualified root alias uses human output, just like version.
	dir := t.TempDir()
	request := runRequest{Binary: binary, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096, Args: []string{"version"}}
	command, err := runProcess(request)
	if err != nil {
		t.Fatal(err)
	}
	request.Args = []string{"--version"}
	alias, err := runProcess(request)
	if err != nil {
		t.Fatal(err)
	}
	if err := compareProcess(processExpectation{Got: alias, Want: command}); err != nil {
		t.Fatal(err)
	}
}

func TestSubjectMachineMissingCommandsRemainErrors(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, mode := range []string{"json", "id", "none"} {
		t.Run(mode, func(t *testing.T) {
			request := runRequest{Binary: binary, Dir: t.TempDir(), Args: []string{"--output", mode}, Timeout: 5 * time.Second, OutputLimit: 4096}
			baseline, err := runProcess(request)
			if err != nil {
				t.Fatal(err)
			}
			if baseline.Status != 2 || len(baseline.Stdout) != 0 || len(baseline.Stderr) == 0 {
				t.Fatalf("missing command must remain a usage error: %+v", baseline)
			}
			if mode == "json" {
				if err := validateSchema(schemaRequest{JSON: baseline.Stderr}); err != nil {
					t.Fatal(err)
				}
			}
			for _, group := range []string{"thread", "message"} {
				request.Args = []string{"--output", mode, group}
				got, err := runProcess(request)
				if err != nil {
					t.Fatal(err)
				}
				if err := compareProcess(processExpectation{Got: got, Want: baseline}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

func TestSubjectMachineSelectorErrorsRemainStable(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	for _, mode := range []string{"id", "none"} {
		t.Run(mode, func(t *testing.T) {
			got, err := runProcess(runRequest{Binary: binary, Dir: t.TempDir(), Args: []string{"--output", mode, "--output", "text", "version"}, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 2, Stderr: []byte("error usage usage:output:repeated\n")}}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
