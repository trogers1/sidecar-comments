// Package runner executes Sidecar v1 fixtures solely through a child executable.
package runner

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type Config struct {
	CorpusPath, Binary, CLIVersion string
	Timeout                        time.Duration
	Stdout, Stderr                 io.Writer
	protocolValidator              *protocolValidator
}
type Report struct {
	Passed, Failed int
	Failures       []Failure
}

type FailureClass string

const (
	FailureArtifact    FailureClass = "artifact"
	FailureCapability  FailureClass = "capability"
	FailureConformance FailureClass = "conformance"
	FailureProcess     FailureClass = "process"
	FailureTimeout     FailureClass = "timeout"
)

type Failure struct {
	ID     string
	Class  FailureClass
	Reason string
}

type corpus struct {
	CorpusVersion int                    `json:"corpus_version"`
	Schema        string                 `json:"schema"`
	Protocol      string                 `json:"protocol_schema"`
	Runner        runnerSpec             `json:"runner"`
	Requirements  map[string]requirement `json:"requirements"`
	Fixtures      []fixtureRef           `json:"fixtures"`
}
type runnerSpec struct {
	DefaultBin      string            `json:"default_bin"`
	EnvBin          string            `json:"env_bin"`
	CLIVersionEnv   string            `json:"cli_version_env"`
	Comparison      string            `json:"comparison"`
	TemplateVersion int               `json:"template_version"`
	Templates       map[string]string `json:"templates"`
}
type requirement struct {
	Source  string `json:"source"`
	Subject string `json:"subject"`
}
type fixtureRef struct {
	ID           string   `json:"id"`
	Golden       string   `json:"golden"`
	Requirements []string `json:"requirements"`
	Coverage     []string `json:"coverage"`
}
type golden struct {
	FixtureVersion int        `json:"fixture_version"`
	ID             string     `json:"id"`
	Requirements   []string   `json:"requirements"`
	Argv           []string   `json:"argv"`
	Env            []envEntry `json:"env"`
	Stdin          string     `json:"stdin_b64"`
	Requires       []string   `json:"requires"`
	Initial        fsSpec     `json:"initial_fs"`
	Expect         expected   `json:"expect"`
}
type expected struct {
	Status          *int    `json:"status"`
	Stdout          *string `json:"stdout_b64"`
	StdoutTemplate  *string `json:"stdout_template"`
	Stderr          *string `json:"stderr_b64"`
	StderrTemplate  *string `json:"stderr_template"`
	InvocationError string  `json:"invocation_error"`
	Final           fsSpec  `json:"final_fs"`
}
type envEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
type fsSpec struct {
	Entries []entry `json:"entries"`
}
type entry struct {
	Path          string `json:"path"`
	Kind          string `json:"kind"`
	Bytes         string `json:"bytes_b64"`
	BytesTemplate string `json:"bytes_template"`
	Target        string `json:"target"`
	Mode          string `json:"mode"`
}

type materializeRequest struct {
	Root string
	FS   fsSpec
}
type assertFSRequest struct {
	Root     string
	Want     fsSpec
	Bindings *uuidBindings
}
type expandRequest struct {
	Initial    fsSpec
	Final      fsSpec
	CLIVersion string
}
type commandRequest struct {
	Command *exec.Cmd
	Timeout time.Duration
}
type expectedBytesRequest struct {
	Encoded   *string
	Template  *string
	Expansion expandRequest
}

const (
	maxChildStreamBytes = 8 << 20
	commandWaitDelay    = 250 * time.Millisecond
)

type boundedBuffer struct {
	bytes.Buffer
	limit    int
	command  *exec.Cmd
	overflow bool
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	if buffer.overflow || buffer.Len()+len(data) > buffer.limit {
		remaining := buffer.limit - buffer.Len()
		if remaining > 0 {
			_, _ = buffer.Buffer.Write(data[:remaining])
		}
		buffer.overflow = true
		if buffer.command != nil && buffer.command.Process != nil {
			_ = buffer.command.Process.Kill()
		}
		return 0, errors.New("child output exceeds capture limit")
	}
	return buffer.Buffer.Write(data)
}

func decodeCorpus(path string) (corpus, error) {
	var out corpus
	b, err := readUnique(path)
	if err != nil {
		return out, err
	}
	return out, decodeJSONCorpus(b, &out, path)
}
func decodeGolden(path string) (golden, error) {
	var out golden
	b, err := readUnique(path)
	if err != nil {
		return out, err
	}
	return out, decodeJSONGolden(b, &out, path)
}
func readUnique(path string) ([]byte, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err = uniqueJSON(b); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return b, nil
}

func ValidateArtifacts(corpusPath string) error {
	if corpusPath == "" {
		corpusPath = "contracts/corpus/v1.json"
	}
	corpusBytes, err := readUnique(corpusPath)
	if err != nil {
		return err
	}
	var c corpus
	if err = decodeJSONCorpus(corpusBytes, &c, corpusPath); err != nil {
		return err
	}
	base, err := repoRoot(corpusPath)
	if err != nil {
		return err
	}
	if err = validateJSONSchemas(schemaValidationRequest{Base: base, Corpus: corpusBytes, ParsedCorpus: c, CorpusPath: corpusPath}); err != nil {
		return err
	}
	if err = validateCorpus(base, c); err != nil {
		return err
	}
	if _, err = validateRequiredRedRegistry(base); err != nil {
		return err
	}
	if err := validateBehaviorInventory(base, c); err != nil {
		return err
	}
	if err := ValidateObligationLedgerV2(base); err != nil {
		return err
	}
	return ValidateV2CapabilityDesign(base)
}
func Run(c Config) (Report, error) {
	if c.CorpusPath == "" {
		c.CorpusPath = "contracts/corpus/v1.json"
	}
	if c.Timeout <= 0 {
		c.Timeout = 10 * time.Second
	}
	if c.Stdout == nil {
		c.Stdout = os.Stdout
	}
	if c.Stderr == nil {
		c.Stderr = os.Stderr
	}
	if err := ValidateArtifacts(c.CorpusPath); err != nil {
		return artifactFailureReport(err)
	}
	manifest, err := decodeCorpus(c.CorpusPath)
	if err != nil {
		return artifactFailureReport(err)
	}
	base, err := repoRoot(c.CorpusPath)
	if err != nil {
		return artifactFailureReport(err)
	}
	if c.Binary == "" {
		c.Binary = os.Getenv(manifest.Runner.EnvBin)
	}
	if c.Binary == "" {
		c.Binary = manifest.Runner.DefaultBin
	}
	binary, err := filepath.Abs(c.Binary)
	if err != nil {
		return artifactFailureReport(err)
	}
	c.Binary = binary
	if c.CLIVersion == "" {
		c.CLIVersion = os.Getenv(manifest.Runner.CLIVersionEnv)
	}
	if c.CLIVersion == "" {
		c.CLIVersion = "dev"
	}
	protocolValidator, err := compileProtocolValidator(base)
	if err != nil {
		return artifactFailureReport(err)
	}
	c.protocolValidator = protocolValidator
	var report Report
	for _, ref := range manifest.Fixtures {
		g, err := decodeGolden(filepath.Join(base, ref.Golden))
		if err != nil {
			return report, err
		}
		if err := requireCapabilities(g.Requires); err != nil {
			failure := Failure{ID: g.ID, Class: FailureCapability, Reason: err.Error()}
			report.Failed++
			report.Failures = append(report.Failures, failure)
			fmt.Fprintf(c.Stderr, "UNAVAILABLE %s [%s]: %v\n", g.ID, failure.Class, err)
			continue
		}
		if err := execute(c, g); err != nil {
			failure := Failure{ID: g.ID, Class: classifyFailure(err), Reason: err.Error()}
			report.Failed++
			report.Failures = append(report.Failures, failure)
			fmt.Fprintf(c.Stderr, "FAIL %s [%s]: %v\n", g.ID, failure.Class, err)
		} else {
			report.Passed++
			fmt.Fprintf(c.Stdout, "PASS %s\n", g.ID)
		}
	}
	sort.Slice(report.Failures, func(i, j int) bool { return report.Failures[i].ID < report.Failures[j].ID })
	fmt.Fprintf(c.Stdout, "contracts: %d passed, %d failed\n", report.Passed, report.Failed)
	if report.Failed != 0 {
		return report, errors.New("contract failures")
	}
	return report, nil
}

// artifactFailureReport ensures preflight failures are visible in the same
// report channel as fixture failures. Callers must never be able to interpret
// an unvalidated or otherwise unrun corpus as a successful zero-failure run.
func artifactFailureReport(err error) (Report, error) {
	failure := Failure{ID: "artifacts", Class: FailureArtifact, Reason: err.Error()}
	return Report{Failed: 1, Failures: []Failure{failure}}, err
}

func classifyFailure(err error) FailureClass {
	message := err.Error()
	switch {
	case strings.Contains(message, "timed out"):
		return FailureTimeout
	case strings.Contains(message, "status got"), strings.Contains(message, "stdout"), strings.Contains(message, "stderr"), strings.Contains(message, "envelope"), strings.Contains(message, "filesystem"), strings.Contains(message, "mode"), strings.Contains(message, "hardlink"):
		return FailureConformance
	case strings.Contains(message, "executable"), strings.Contains(message, "fork/exec"), strings.Contains(message, "exit status"), strings.Contains(message, "output exceeds capture limit"):
		return FailureProcess
	default:
		return FailureArtifact
	}
}

func repoRoot(corpusPath string) (string, error) {
	absolute, err := filepath.Abs(corpusPath)
	if err != nil {
		return "", err
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(absolute))), nil
}
func validateCorpus(base string, c corpus) error {
	if c.CorpusVersion != 1 || c.Schema != "contracts/schema/corpus-v1.json" || c.Protocol != "contracts/schema/v1.json" {
		return errors.New("unsupported corpus manifest")
	}
	if c.Runner.DefaultBin == "" || c.Runner.EnvBin != "SIDECAR_BIN" || c.Runner.CLIVersionEnv != "SIDECAR_CLI_VERSION" || c.Runner.Comparison != "byte-exact after approved template expansion" || c.Runner.TemplateVersion != 1 || len(c.Runner.Templates) != 5 {
		return errors.New("invalid runner declaration")
	}
	if c.Runner.Templates["CLI_VERSION"] == "" || c.Runner.Templates["sha256:initial:PATH"] == "" || c.Runner.Templates["sha256:final:PATH"] == "" || c.Runner.Templates["UUIDV7:bind:NAME"] == "" || c.Runner.Templates["UUIDV7:ref:NAME"] == "" {
		return errors.New("incomplete template declaration")
	}
	if len(c.Requirements) == 0 || len(c.Fixtures) == 0 {
		return errors.New("empty corpus")
	}
	covered := make(map[string]bool)
	ids := make(map[string]bool)
	goldens := make(map[string]bool)
	for id, r := range c.Requirements {
		if id == "" || r.Subject == "" || !safeReference(r.Source) {
			return fmt.Errorf("invalid requirement %q", id)
		}
		info, err := os.Stat(filepath.Join(base, r.Source))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("stale requirement source %s", r.Source)
		}
	}
	behaviorIDs := make(map[string]bool)
	for _, ref := range c.Fixtures {
		if ref.ID == "" || ids[ref.ID] || goldens[ref.Golden] || !safeGolden(ref.Golden) || len(ref.Coverage) != 1 || !strings.HasPrefix(ref.Coverage[0], "BHV-") || behaviorIDs[ref.Coverage[0]] {
			return fmt.Errorf("invalid fixture reference %q", ref.ID)
		}
		behaviorIDs[ref.Coverage[0]] = true
		ids[ref.ID] = true
		goldens[ref.Golden] = true
		for _, id := range ref.Requirements {
			if _, ok := c.Requirements[id]; !ok {
				return fmt.Errorf("%s unknown requirement %s", ref.ID, id)
			}
			covered[id] = true
		}
		g, err := decodeGolden(filepath.Join(base, ref.Golden))
		if err != nil {
			return err
		}
		if g.ID != ref.ID || !sameStrings(g.Requirements, ref.Requirements) {
			return fmt.Errorf("fixture/golden contradiction %s", ref.ID)
		}
		if err := validateGolden(g); err != nil {
			return fmt.Errorf("%s: %w", ref.ID, err)
		}
	}
	for id := range c.Requirements {
		if !covered[id] {
			return fmt.Errorf("orphan requirement %s", id)
		}
	}
	entries, err := os.ReadDir(filepath.Join(base, "contracts/goldens/v1"))
	if err != nil {
		return err
	}
	for _, item := range entries {
		if item.IsDir() {
			return fmt.Errorf("unexpected golden directory %s", item.Name())
		}
		if strings.HasSuffix(item.Name(), ".json") && !goldens["contracts/goldens/v1/"+item.Name()] {
			return fmt.Errorf("orphan golden %s", item.Name())
		}
	}
	return nil
}
func validateGolden(g golden) error {
	if g.FixtureVersion != 1 || g.ID == "" || len(g.Requirements) == 0 || g.Argv == nil || g.Env == nil || g.Requires == nil || g.Initial.Entries == nil || g.Expect.Final.Entries == nil || g.Expect.Status == nil || *g.Expect.Status < 0 || *g.Expect.Status > 8 {
		return errors.New("invalid golden shape")
	}
	if (g.Expect.Stdout == nil) == (g.Expect.StdoutTemplate == nil) {
		return errors.New("expected stdout must use exactly one mode")
	}
	if (g.Expect.Stderr == nil) == (g.Expect.StderrTemplate == nil) {
		return errors.New("expected stderr must use exactly one mode")
	}
	if g.Expect.InvocationError != "" && g.Expect.InvocationError != "argv" {
		return errors.New("invalid invocation error declaration")
	}
	encodedValues := []string{g.Stdin}
	if g.Expect.Stderr != nil {
		encodedValues = append(encodedValues, *g.Expect.Stderr)
	}
	if g.Expect.Stdout != nil {
		encodedValues = append(encodedValues, *g.Expect.Stdout)
	}
	for _, encoded := range encodedValues {
		if _, err := base64.StdEncoding.DecodeString(encoded); err != nil {
			return errors.New("invalid base64")
		}
	}
	envNames := make(map[string]bool)
	for _, variable := range g.Env {
		if variable.Name == "" || strings.Contains(variable.Name, "=") || envNames[variable.Name] {
			return errors.New("invalid or duplicate environment name")
		}
		envNames[variable.Name] = true
	}
	if err := validateFS(g.Initial); err != nil {
		return fmt.Errorf("initial_fs: %w", err)
	}
	for _, e := range g.Initial.Entries {
		if e.BytesTemplate != "" {
			return errors.New("initial_fs bytes_template is forbidden")
		}
	}
	if err := validateFS(g.Expect.Final); err != nil {
		return fmt.Errorf("final_fs: %w", err)
	}
	if err := validateUUIDTemplates(g); err != nil {
		return fmt.Errorf("UUIDV7 templates: %w", err)
	}
	stdout, err := expectedBytes(expectedBytesRequest{Encoded: g.Expect.Stdout, Template: g.Expect.StdoutTemplate, Expansion: expandRequest{Initial: g.Initial, Final: g.Expect.Final, CLIVersion: "artifact"}})
	if err != nil {
		return fmt.Errorf("stdout expectation: %w", err)
	}
	stderr, err := expectedBytes(expectedBytesRequest{Encoded: g.Expect.Stderr, Template: g.Expect.StderrTemplate, Expansion: expandRequest{Initial: g.Initial, Final: g.Expect.Final, CLIVersion: "artifact"}})
	if err != nil {
		return fmt.Errorf("stderr expectation: %w", err)
	}
	if err := validateEnvelopes(g, *g.Expect.Status, stdout, stderr); err != nil {
		return fmt.Errorf("illegal command/output/status/stream declaration: %w", err)
	}
	for _, r := range g.Requires {
		if !knownCapability(r) {
			return fmt.Errorf("unknown mandatory capability %q", r)
		}
	}
	return nil
}
func validateFS(f fsSpec) error {
	seen := make(map[string]bool)
	for _, e := range f.Entries {
		if !safePath(e.Path) || strings.Contains(e.Path, "{{") || strings.Contains(e.Target, "{{") || seen[e.Path] {
			return fmt.Errorf("unsafe or duplicate path %q", e.Path)
		}
		seen[e.Path] = true
		if e.Mode != "" && e.Mode != "0000" && e.Mode != "0600" && e.Mode != "0644" && e.Mode != "0755" {
			return errors.New("invalid mode")
		}
		if (e.Kind == "file" || e.Kind == "directory") && e.Mode == "" {
			return errors.New("regular filesystem objects require an exact mode")
		}
		switch e.Kind {
		case "file":
			if e.Target != "" || (e.Bytes != "" && e.BytesTemplate != "") {
				return errors.New("file payload")
			}
		case "directory":
			if e.Target != "" || e.Bytes != "" || e.BytesTemplate != "" {
				return errors.New("directory payload")
			}
		case "symlink", "hardlink":
			if !safePath(e.Target) || e.Bytes != "" || e.BytesTemplate != "" || e.Mode != "" {
				return errors.New("invalid link declaration")
			}
		default:
			return fmt.Errorf("unsupported filesystem kind %q", e.Kind)
		}
		if e.Bytes != "" {
			if _, err := base64.StdEncoding.DecodeString(e.Bytes); err != nil {
				return err
			}
		}
	}
	return nil
}
func safeReference(p string) bool { return safePath(p) }
func safeGolden(p string) bool {
	return safePath(p) && strings.HasPrefix(p, "contracts/goldens/v1/") && strings.HasSuffix(p, ".json")
}

// safePath performs the semantic checks that JSON Schema regular expressions
// cannot portably express. The schema limits the alphabet; this typed check
// rejects absolute paths and every parent-directory segment.
func safePath(p string) bool {
	if p == "" || p == "." || filepath.IsAbs(p) || filepath.Clean(p) != p {
		return false
	}
	for _, component := range strings.Split(filepath.ToSlash(p), "/") {
		if component == ".." {
			return false
		}
	}
	return true
}
func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

type CapabilityError struct {
	Capability string
	Reason     string
}

func (err CapabilityError) Error() string {
	return fmt.Sprintf("mandatory capability %s unavailable: %s", err.Capability, err.Reason)
}

type capability struct {
	name  string
	check func() error
}

// capabilityFor is deliberately a closed, typed registry rather than a list of
// advisory strings. A fixture is run only after its required harness feature
// has been proved usable; unavailable is a reportable nonzero result.
func capabilityFor(name string) (capability, bool) {
	switch name {
	case "writer_platform":
		return capability{name: name, check: checkWriterPlatform}, true
	case "symlink":
		return capability{name: name, check: checkSymlink}, true
	case "directory":
		return capability{name: name, check: checkDirectory}, true
	case "hardlink":
		return capability{name: name, check: checkHardlink}, true
	case "permission":
		return capability{name: name, check: checkPermission}, true
	case "stdin":
		return capability{name: name, check: checkStdin}, true
	case "process":
		return capability{name: name, check: checkProcess}, true
	case "fault_injection":
		return capability{name: name, check: checkFaultInjection}, true
	case "large_input":
		return capability{name: name, check: checkLargeInput}, true
	case "deterministic_blob_limit_input":
		return capability{name: name, check: func() error {
			return errors.New("deterministic blob/limit input adapter is not implemented by the pre-implementation harness")
		}}, true
	case "process_orchestration":
		return capability{name: name, check: func() error {
			return errors.New("barrier-controlled process orchestration/readiness adapter is not implemented by the pre-implementation harness")
		}}, true
	case "held_lock_identity_replacement":
		return capability{name: name, check: func() error {
			return errors.New("held-lock and identity-replacement adapter is not implemented by the pre-implementation harness")
		}}, true
	case "platform_control":
		return capability{name: name, check: func() error {
			return errors.New("controlled non-writer platform adapter is not implemented by the pre-implementation harness")
		}}, true
	default:
		return capability{}, false
	}
}
func knownCapability(name string) bool {
	_, ok := capabilityFor(name)
	return ok
}
func requireCapabilities(names []string) error {
	for _, name := range names {
		feature, ok := capabilityFor(name)
		if !ok {
			return CapabilityError{Capability: name, Reason: "unknown capability"}
		}
		if err := feature.check(); err != nil {
			return CapabilityError{Capability: feature.name, Reason: err.Error()}
		}
	}
	return nil
}
func checkWriterPlatform() error {
	if writerPlatform() {
		return nil
	}
	return fmt.Errorf("unsupported on %s/%s", runtime.GOOS, runtime.GOARCH)
}
func capabilityDirectory() (string, error) { return os.MkdirTemp("", "sidecar-capability-") }
func checkSymlink() error {
	dir, err := capabilityDirectory()
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	return os.Symlink("target", filepath.Join(dir, "link"))
}
func checkDirectory() error {
	dir, err := capabilityDirectory()
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	return os.Mkdir(filepath.Join(dir, "child"), 0755)
}
func checkHardlink() error {
	dir, err := capabilityDirectory()
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	original := filepath.Join(dir, "original")
	if err = os.WriteFile(original, nil, 0600); err != nil {
		return err
	}
	return os.Link(original, filepath.Join(dir, "linked"))
}
func checkPermission() error {
	dir, err := capabilityDirectory()
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "mode")
	if err = os.WriteFile(path, nil, 0600); err != nil {
		return err
	}
	if err = os.Chmod(path, 0000); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0000 {
		return fmt.Errorf("permission assertion failed: got %04o want 0000", info.Mode().Perm())
	}
	file, err := os.Open(path)
	if err == nil {
		_ = file.Close()
		return errors.New("permission denial is not enforced")
	}
	if !errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("permission probe returned unexpected error: %w", err)
	}
	return nil
}
func checkStdin() error {
	input := bytes.NewReader([]byte("stdin"))
	observed, err := io.ReadAll(input)
	if err != nil {
		return err
	}
	if !bytes.Equal(observed, []byte("stdin")) {
		return errors.New("runner stdin transport changed bytes")
	}
	return nil
}
func checkProcess() error {
	_, err := os.Executable()
	return err
}
func checkFaultInjection() error {
	return errors.New("controlled fault injection is not implemented by the pre-implementation harness")
}
func checkLargeInput() error {
	data := make([]byte, 1<<20)
	if len(data) != 1<<20 {
		return errors.New("runner cannot allocate the documented large-input probe")
	}
	return nil
}

func writerPlatform() bool {
	return (runtime.GOOS == "darwin" && runtime.GOARCH == "arm64") || (runtime.GOOS == "linux" && runtime.GOARCH == "amd64")
}
func execute(c Config, g golden) error {
	root, err := os.MkdirTemp("", "sidecar-contract-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	if err = materialize(materializeRequest{Root: root, FS: g.Initial}); err != nil {
		return err
	}
	in, _ := base64.StdEncoding.DecodeString(g.Stdin)
	cmd := exec.Command(c.Binary, g.Argv...)
	cmd.Dir = root
	cmd.Env = exactEnvironment(g.Env)
	cmd.Stdin = bytes.NewReader(in)
	stdout := boundedBuffer{limit: maxChildStreamBytes, command: cmd}
	stderr := boundedBuffer{limit: maxChildStreamBytes, command: cmd}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := runCommand(commandRequest{Command: cmd, Timeout: c.Timeout})
	if stdout.overflow || stderr.overflow {
		return errors.New("child output exceeds capture limit")
	}
	status := 0
	var exit *exec.ExitError
	if errors.As(runErr, &exit) {
		status = exit.ExitCode()
	} else if runErr != nil {
		return runErr
	}
	wantOut, err := runtimeExpectedTemplate(g.Expect.Stdout, g.Expect.StdoutTemplate, expandRequest{Initial: g.Initial, Final: g.Expect.Final, CLIVersion: c.CLIVersion})
	if err != nil {
		return err
	}
	wantErr, err := runtimeExpectedTemplate(g.Expect.Stderr, g.Expect.StderrTemplate, expandRequest{Initial: g.Initial, Final: g.Expect.Final, CLIVersion: c.CLIVersion})
	if err != nil {
		return err
	}
	bindings := newUUIDBindings()
	if status != *g.Expect.Status {
		return fmt.Errorf("status got %d want %d", status, *g.Expect.Status)
	}
	if err := matchExpectedStream(wantOut, stdout.Bytes(), g.Expect.StdoutTemplate != nil, bindings, g.Initial, g.Expect.Final); err != nil {
		return fmt.Errorf("stdout bytes differ: %w", err)
	}
	if err := matchExpectedStream(wantErr, stderr.Bytes(), g.Expect.StderrTemplate != nil, bindings, g.Initial, g.Expect.Final); err != nil {
		return fmt.Errorf("stderr bytes differ: %w", err)
	}
	if err := validateEnvelopes(g, status, stdout.Bytes(), stderr.Bytes()); err != nil {
		return err
	}
	if c.protocolValidator != nil {
		if jsonOutput(g.Argv) {
			if err := c.protocolValidator.validateActual(stdout.Bytes(), stderr.Bytes()); err != nil {
				return err
			}
		} else if invocation, invocationErr := parseInvocation(g.Argv); invocationErr == nil && invocation.output == "text" && len(stdout.Bytes()) != 0 {
			textEnvelope, textErr := textSuccessEnvelope(stdout.Bytes(), invocation.command)
			if textErr != nil {
				return textErr
			}
			if textErr = validateSchemaInstance(c.protocolValidator.schema, textEnvelope); textErr != nil {
				return fmt.Errorf("child text result schema validation: %w", textErr)
			}
		}
	}
	return assertFS(assertFSRequest{Root: root, Want: g.Expect.Final, Bindings: bindings})
}
func matchExpectedStream(expected, actual []byte, templated bool, bindings *uuidBindings, initial, final fsSpec) error {
	if !templated {
		if !bytes.Equal(expected, actual) {
			return errors.New("bytes differ")
		}
		return nil
	}
	return matchRuntimeTemplate(expected, actual, bindings, initial, final)
}
func exactEnvironment(entries []envEntry) []string {
	environment := make([]string, 0, len(entries))
	for _, entry := range entries {
		environment = append(environment, entry.Name+"="+entry.Value)
	}
	return environment
}
func runCommand(request commandRequest) error {
	// WaitDelay closes inherited stdio pipes after the child exits or is killed.
	// This bounds Wait when a descendant retains a pipe after its direct parent
	// has timed out. It cannot portably terminate that independent descendant.
	request.Command.WaitDelay = commandWaitDelay
	// Start synchronously: Process must be published before the timeout path
	// reads it or calls Kill. Only Wait runs concurrently with the timer.
	if err := request.Command.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- request.Command.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(request.Timeout):
		_ = request.Command.Process.Kill()
		<-done
		return fmt.Errorf("timed out after %s", request.Timeout)
	}
}
func expectedBytes(request expectedBytesRequest) ([]byte, error) {
	out, err := runtimeExpectedBytes(request)
	if err != nil || request.Template == nil {
		return out, err
	}
	return renderUUIDTemplatesForValidation(out)
}
func runtimeExpectedBytes(request expectedBytesRequest) ([]byte, error) {
	if request.Template != nil {
		return expand([]byte(*request.Template), request.Expansion)
	}
	if request.Encoded == nil {
		return nil, errors.New("missing stream expectation")
	}
	return base64.StdEncoding.DecodeString(*request.Encoded)
}

// runtimeExpectedTemplate deliberately leaves revision tokens intact. A final
// file may contain a UUID captured earlier in stdout, so its digest cannot be
// known until that binding has been observed.
func runtimeExpectedTemplate(encoded, template *string, request expandRequest) ([]byte, error) {
	if template == nil {
		if encoded == nil {
			return nil, errors.New("missing stream expectation")
		}
		return base64.StdEncoding.DecodeString(*encoded)
	}
	// SHA tokens intentionally remain for matchRuntimeTemplate; UUID placement
	// was already validated while loading the artifact.
	return bytes.ReplaceAll([]byte(*template), []byte("{{CLI_VERSION}}"), []byte(request.CLIVersion)), nil
}
func expand(input []byte, request expandRequest) ([]byte, error) {
	out := append([]byte(nil), input...)
	out = bytes.ReplaceAll(out, []byte("{{CLI_VERSION}}"), []byte(request.CLIVersion))
	for _, fs := range []struct {
		Prefix string
		FS     fsSpec
	}{{"initial", request.Initial}, {"final", request.Final}} {
		for _, e := range fs.FS.Entries {
			if e.Kind != "file" {
				continue
			}
			data, err := base64.StdEncoding.DecodeString(e.Bytes)
			if err != nil {
				return nil, err
			}
			if e.BytesTemplate != "" {
				data, err = renderUUIDTemplatesForValidation([]byte(e.BytesTemplate))
				if err != nil {
					return nil, err
				}
			}
			token := []byte("{{sha256:" + fs.Prefix + ":" + e.Path + "}}")
			sum := sha256.Sum256(data)
			out = bytes.ReplaceAll(out, token, []byte("sha256:"+fmt.Sprintf("%x", sum)))
		}
	}
	if bytes.Contains(out, []byte("{{")) {
		if err := validateUUIDTokenSyntax(out); err != nil {
			return nil, errors.New("unexpanded or undocumented template")
		}
	}
	return out, nil
}
func materialize(request materializeRequest) error {
	// Fixed phases make fixtures reproducible even when JSON entry order changes:
	// declared directories, regular files, then links. Parent directories are
	// created with a stable private-independent mode and are never inferred as
	// asserted fixture entries.
	for _, kind := range []string{"directory", "file", "hardlink", "symlink"} {
		for _, e := range request.FS.Entries {
			if e.Kind != kind {
				continue
			}
			path := filepath.Join(request.Root, e.Path)
			if err := ensureMaterializationParents(request.Root, filepath.Dir(path)); err != nil {
				return err
			}
			data, err := base64.StdEncoding.DecodeString(e.Bytes)
			if err != nil {
				return err
			}
			switch e.Kind {
			case "file":
				err = os.WriteFile(path, data, fileMode(e.Mode))
			case "directory":
				err = os.MkdirAll(path, fileMode(e.Mode))
			case "symlink":
				err = os.Symlink(e.Target, path)
			case "hardlink":
				target := filepath.Join(request.Root, e.Target)
				info, statErr := os.Lstat(target)
				if statErr != nil {
					return statErr
				}
				if !info.Mode().IsRegular() {
					return fmt.Errorf("hardlink target is not a regular file: %s", e.Target)
				}
				err = os.Link(target, path)
			}
			if err != nil {
				return err
			}
			if e.Kind == "file" || e.Kind == "directory" {
				if err = os.Chmod(path, fileMode(e.Mode)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// ensureMaterializationParents creates only undeclared implicit parents. Their
// mode is corrected after Mkdir so fixture setup is independent of the host
// umask. Existing declared directories are never modified here; their declared
// mode is set in the directory phase.
func ensureMaterializationParents(root, parent string) error {
	relative, err := filepath.Rel(root, parent)
	if err != nil {
		return err
	}
	if relative == "." {
		return nil
	}
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, component)
		err = os.Mkdir(current, 0755)
		if err == nil {
			if err = os.Chmod(current, 0755); err != nil {
				return err
			}
			continue
		}
		if !errors.Is(err, os.ErrExist) {
			return err
		}
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return statErr
		}
		if !info.IsDir() {
			return fmt.Errorf("materialization parent is not a directory: %s", current)
		}
	}
	return nil
}
func fileMode(s string) os.FileMode {
	switch s {
	case "0000":
		return 0
	case "0600":
		return 0600
	case "0755":
		return 0755
	}
	return 0644
}
func assertFS(request assertFSRequest) error {
	want := make(map[string]entry, len(request.Want.Entries))
	for _, e := range request.Want.Entries {
		want[e.Path] = e
	}
	seen := make(map[string]bool)
	err := filepath.WalkDir(request.Root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == request.Root {
			return nil
		}
		relative, _ := filepath.Rel(request.Root, path)
		relative = filepath.ToSlash(relative)
		e, ok := want[relative]
		if !ok {
			if d.IsDir() && implicitParent(relative, want) {
				return nil
			}
			return fmt.Errorf("unexpected filesystem entry %s", relative)
		}
		seen[relative] = true
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if e.Mode != "" && fmt.Sprintf("%04o", info.Mode().Perm()) != e.Mode {
			return fmt.Errorf("filesystem mode differs for %s", relative)
		}
		switch e.Kind {
		case "file":
			if !info.Mode().IsRegular() {
				return fmt.Errorf("filesystem type differs for %s", relative)
			}
			data, err := readAssertFile(path, info.Mode().Perm())
			if err != nil {
				return err
			}
			if e.BytesTemplate != "" {
				if request.Bindings == nil {
					return fmt.Errorf("filesystem UUIDV7 template has no bindings for %s", relative)
				}
				if err := matchUUIDTemplate([]byte(e.BytesTemplate), data, request.Bindings); err != nil {
					return fmt.Errorf("filesystem bytes differ for %s: %w", relative, err)
				}
			} else if base64.StdEncoding.EncodeToString(data) != e.Bytes {
				return fmt.Errorf("filesystem bytes differ for %s", relative)
			}
		case "directory":
			if !info.IsDir() {
				return fmt.Errorf("filesystem type differs for %s", relative)
			}
		case "symlink":
			if info.Mode()&os.ModeSymlink == 0 {
				return fmt.Errorf("filesystem type differs for %s", relative)
			}
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if target != e.Target {
				return fmt.Errorf("symlink target differs for %s", relative)
			}
		case "hardlink":
			other, err := os.Stat(filepath.Join(request.Root, e.Target))
			if err != nil {
				return err
			}
			if !os.SameFile(info, other) {
				return fmt.Errorf("hardlink target differs for %s", relative)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(seen) != len(want) {
		return errors.New("expected filesystem entry missing")
	}
	return nil
}

// readAssertFile temporarily restores owner read/write access only after the
// final mode and regular-file type have been checked. This lets final-state
// assertions inspect a deliberately unchanged 0000 file while restoring that
// asserted mode even when reading the file fails.
func readAssertFile(path string, mode os.FileMode) (data []byte, err error) {
	if mode != 0 {
		return os.ReadFile(path)
	}
	if err = os.Chmod(path, 0600); err != nil {
		return nil, fmt.Errorf("temporarily enabling read access for 0000 assertion: %w", err)
	}
	defer func() {
		if restoreErr := os.Chmod(path, mode); restoreErr != nil && err == nil {
			err = fmt.Errorf("restoring asserted mode %04o: %w", mode, restoreErr)
		}
	}()
	return os.ReadFile(path)
}

func implicitParent(path string, want map[string]entry) bool {
	prefix := path + "/"
	for child := range want {
		if strings.HasPrefix(child, prefix) {
			return true
		}
	}
	return false
}
