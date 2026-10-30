package cli_test

import (
	"testing"
	"time"
)

// A second global selector is rejected; it cannot override error rendering.
func TestSubjectRepeatedOutput(t *testing.T) {
	binary := subjectBinary(subjectRequest{T: t})
	failure := `{"protocol":1,"error":{"code":"usage","message":"usage:output:repeated","details":{"field":"output","reason":"repeated"}}}` + "\n"
	if err := validateSchema(schemaRequest{JSON: []byte(failure)}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		json bool
	}{
		{name: "separate_text", args: []string{"--output", "text", "--output", "json", "version"}, json: false},
		{name: "equals_text", args: []string{"--output=text", "--output=json", "version"}, json: false},
		{name: "separate_json", args: []string{"--output", "json", "--output", "text", "version"}, json: true},
		{name: "equals_json", args: []string{"--output=json", "--output=text", "version"}, json: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			got, err := runProcess(runRequest{Binary: binary, Args: tc.args, Dir: dir, Timeout: 5 * time.Second, OutputLimit: 4096})
			if err != nil {
				t.Fatal(err)
			}
			want := "error usage usage:output:repeated\n"
			if tc.json {
				want = failure
			}
			if !tc.json {
				assertHumanOutput(humanOutputExpectation{T: t, Got: got, Status: 2, Error: true, Terms: []string{"--output", "--help"}})
			} else if err := compareProcess(processExpectation{Got: got, Want: processResult{Status: 2, Stderr: []byte(want)}}); err != nil {
				t.Fatal(err)
			}
			if tc.json {
				if err := validateSchema(schemaRequest{JSON: got.Stderr}); err != nil {
					t.Fatal(err)
				}
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
}
