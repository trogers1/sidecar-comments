package runner

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// requiredRedRegistry is the central, machine-checked ledger for isolated
// pre-implementation packs. A scenario is intentionally still red only when
// it has no direct corpus fixture; manifest_ref and golden_ref retain the
// original evidence instead of inventing a runnable expectation.
type requiredRedRegistry struct {
	RequiredRedVersion int                   `json:"required_red_version"`
	CapabilitySchema   string                `json:"capability_schema"`
	Scenarios          []requiredRedScenario `json:"scenarios"`
}
type requiredRedScenario struct {
	ID           string   `json:"id"`
	Pack         string   `json:"pack"`
	ManifestRef  string   `json:"manifest_ref"`
	GoldenRef    string   `json:"golden_ref"`
	Capabilities []string `json:"capabilities"`
}
type capabilityCatalog struct {
	CapabilityVersion  int                `json:"capability_version"`
	UnavailableOutcome unavailableOutcome `json:"unavailable_outcome"`
	Capabilities       []capabilitySpec   `json:"capabilities"`
}
type unavailableOutcome struct {
	Result       string   `json:"result"`
	FailureClass string   `json:"failure_class"`
	Nonzero      bool     `json:"nonzero"`
	Never        []string `json:"never"`
}
type capabilitySpec struct {
	ID                string   `json:"id"`
	Inputs            []string `json:"inputs"`
	Guarantees        []string `json:"guarantees"`
	UnavailableReason string   `json:"unavailable_reason"`
}

func decodeRequiredRedRegistry(path string) (requiredRedRegistry, error) {
	var registry requiredRedRegistry
	b, err := readUnique(path)
	if err != nil {
		return registry, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&registry); err != nil {
		return registry, fmt.Errorf("%s: %w", path, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return registry, fmt.Errorf("%s: trailing JSON", path)
	}
	return registry, nil
}
func decodeCapabilityCatalog(path string) (capabilityCatalog, error) {
	var catalog capabilityCatalog
	b, err := readUnique(path)
	if err != nil {
		return catalog, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(b)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&catalog); err != nil {
		return catalog, fmt.Errorf("%s: %w", path, err)
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return catalog, fmt.Errorf("%s: trailing JSON", path)
	}
	return catalog, nil
}

func validateRequiredRedRegistry(base string) (requiredRedRegistry, error) {
	registry, err := decodeRequiredRedRegistry(filepath.Join(base, "contracts/packs/required-red-v1.json"))
	if err != nil {
		return registry, err
	}
	if registry.RequiredRedVersion != 1 || registry.CapabilitySchema != "contracts/packs/capabilities-v1.json" || len(registry.Scenarios) == 0 {
		return registry, fmt.Errorf("invalid required-red registry header")
	}
	catalog, err := decodeCapabilityCatalog(filepath.Join(base, registry.CapabilitySchema))
	if err != nil {
		return registry, err
	}
	known := map[string]string{}
	if catalog.CapabilityVersion != 1 || catalog.UnavailableOutcome.Result != "UNAVAILABLE" || catalog.UnavailableOutcome.FailureClass != "capability" || !catalog.UnavailableOutcome.Nonzero || !sameStrings(catalog.UnavailableOutcome.Never, []string{"skip", "pass"}) {
		return registry, fmt.Errorf("invalid pack capability catalog")
	}
	for _, spec := range catalog.Capabilities {
		if spec.ID == "" || len(spec.Inputs) == 0 || len(spec.Guarantees) == 0 || spec.UnavailableReason == "" || known[spec.ID] != "" {
			return registry, fmt.Errorf("invalid pack capability %q", spec.ID)
		}
		known[spec.ID] = spec.UnavailableReason
	}
	// Runner-shaped pack goldens are checked against the central corpus and
	// protocol schemas during artifact validation. This is intentionally not an
	// execution or promotion of a required-red scenario.
	runnerGoldenValidators, err := compilePackGoldenValidators(base)
	if err != nil {
		return registry, err
	}
	seen, goldenRefs := map[string]bool{}, map[string]bool{}
	for _, scenario := range registry.Scenarios {
		if scenario.ID == "" || scenario.Pack == "" || scenario.ManifestRef == "" || seen[scenario.ID] {
			return registry, fmt.Errorf("invalid or duplicate required-red scenario %q", scenario.ID)
		}
		seen[scenario.ID] = true
		if err := validateManifestReference(base, scenario); err != nil {
			return registry, err
		}
		if scenario.GoldenRef != "" {
			if !safeReference(scenario.GoldenRef) || goldenRefs[scenario.GoldenRef] {
				return registry, fmt.Errorf("invalid or duplicate pack golden reference %s", scenario.GoldenRef)
			}
			goldenRefs[scenario.GoldenRef] = true
			goldenPath := filepath.Join(base, scenario.GoldenRef)
			if info, statErr := os.Stat(goldenPath); statErr != nil || !info.Mode().IsRegular() {
				return registry, fmt.Errorf("stale pack golden reference %s", scenario.GoldenRef)
			}
			if err := validatePackGolden(goldenPath, scenario.ID, scenario.Pack, runnerGoldenValidators); err != nil {
				return registry, err
			}
		}
		for _, name := range scenario.Capabilities {
			if _, ok := known[name]; !ok && !knownCapability(name) {
				return registry, fmt.Errorf("%s has unknown capability %s", scenario.ID, name)
			}
		}
	}
	// Every checked-in pack golden must be registered exactly once. This makes a
	// new isolated fixture a full-red failure until it has an explicit outcome.
	for _, pack := range []string{"p6", "p8-p10", "p11-p12"} {
		entries, readErr := os.ReadDir(filepath.Join(base, "contracts/packs", pack, "goldens"))
		if readErr != nil {
			return registry, readErr
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			ref := "contracts/packs/" + pack + "/goldens/" + entry.Name()
			if !goldenRefs[ref] {
				return registry, fmt.Errorf("orphaned pack scenario golden %s", ref)
			}
		}
	}
	return registry, nil
}

func validatePackGolden(path, scenarioID, pack string, runnerGoldenValidators *packGoldenValidators) error {
	data, err := readUnique(path)
	if err != nil {
		return err
	}
	// Only P6 and the runner-shaped P11/P12 seeds are corpus fixtures. The
	// remaining pack artifacts are declarative DTOs, not executable fixtures.
	if strings.Contains(string(data), `"fixture_version"`) {
		fixture, err := validateRunnerPackGolden(path, data, runnerGoldenValidators)
		if err != nil {
			return err
		}
		if err = validateGolden(fixture); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		return nil
	}
	switch pack {
	case "p8-p10":
		return validateP8P10Golden(data, path, scenarioID)
	case "p11-p12":
		return validateP11P12DeclarativeGolden(data, path, scenarioID)
	default:
		return fmt.Errorf("%s: unrecognized declarative pack golden", path)
	}
}

// validateManifestReference resolves, rather than merely checks, a registry
// fragment. JSON references are limited to the native arrays used by the two
// JSON manifests; Markdown anchors resolve to the ID cell of a matrix row.
func validateManifestReference(base string, scenario requiredRedScenario) error {
	path, fragment, ok := strings.Cut(scenario.ManifestRef, "#")
	if !ok || !safeReference(path) || fragment == "" {
		return fmt.Errorf("invalid manifest reference %s", scenario.ManifestRef)
	}
	manifestPath := filepath.Join(base, path)
	var data []byte
	var err error
	if strings.HasSuffix(path, ".md") {
		data, err = os.ReadFile(manifestPath)
	} else {
		data, err = readUnique(manifestPath)
	}
	if err != nil {
		return fmt.Errorf("stale manifest reference %s: %w", scenario.ManifestRef, err)
	}
	switch scenario.Pack {
	case "p6":
		var manifest p6Manifest
		if err := decodeP6Manifest(data, &manifest, path); err != nil {
			return err
		}
		value, err := jsonArrayEntry(fragment, map[string][]string{"goldens": manifest.Goldens, "known_gaps": manifest.KnownGaps})
		if err != nil {
			return fmt.Errorf("%s: %w", scenario.ManifestRef, err)
		}
		if scenario.GoldenRef != "" && value != scenario.GoldenRef {
			return fmt.Errorf("%s golden_ref does not match manifest entry", scenario.ID)
		}
	case "p8-p10":
		var manifest p8P10Manifest
		if err := decodeP8P10Manifest(data, &manifest, path); err != nil {
			return err
		}
		index, err := jsonArrayIndex(fragment, "cases", len(manifest.Cases))
		if err != nil {
			return fmt.Errorf("%s: %w", scenario.ManifestRef, err)
		}
		row := manifest.Cases[index]
		if row.ID != scenario.ID {
			return fmt.Errorf("%s scenario ID does not match manifest case %q", scenario.ID, row.ID)
		}
		golden := filepath.ToSlash(filepath.Join(filepath.Dir(path), row.Golden))
		if scenario.GoldenRef != golden {
			return fmt.Errorf("%s golden_ref does not match manifest case", scenario.ID)
		}
		// A registry may declare additional harness capabilities needed to make a
		// prose case observable, but it may not omit a native case requirement.
		if !manifestCapabilitiesDeclared(row.Requires, scenario.Capabilities) {
			return fmt.Errorf("%s omits manifest capability", scenario.ID)
		}
	case "p11-p12":
		row, err := markdownTableRow(data, fragment)
		if err != nil {
			return fmt.Errorf("%s: %w", scenario.ManifestRef, err)
		}
		if row[0] != scenario.ID {
			return fmt.Errorf("%s anchor resolves to %q", scenario.ID, row[0])
		}
		mode := row[len(row)-1]
		if scenario.GoldenRef != "" && mode != "golden" {
			return fmt.Errorf("%s golden_ref disagrees with manifest mode %q", scenario.ID, mode)
		}
		if capability := markdownModeCapability(mode); capability != "" && !manifestCapabilitiesDeclared([]string{capability}, scenario.Capabilities) {
			return fmt.Errorf("%s omits manifest capability %s", scenario.ID, capability)
		}
		if scenario.GoldenRef != "" && !strings.Contains(string(data), "`goldens/"+filepath.Base(scenario.GoldenRef)+"`") {
			return fmt.Errorf("%s golden_ref is not a native seed golden", scenario.ID)
		}
	default:
		return fmt.Errorf("%s has unknown pack %s", scenario.ID, scenario.Pack)
	}
	return nil
}

type p6Manifest struct {
	PackVersion int      `json:"pack_version"`
	ID          string   `json:"id"`
	Goldens     []string `json:"goldens"`
	KnownGaps   []string `json:"known_gaps"`
}
type p8P10Manifest struct {
	PackVersion int         `json:"pack_version"`
	ID          string      `json:"id"`
	Cases       []p8P10Case `json:"cases"`
}
type p8P10Case struct {
	ID          string   `json:"id"`
	Golden      string   `json:"golden"`
	Requires    []string `json:"requires"`
	Assert      string   `json:"assert"`
	Unavailable string   `json:"unavailable"`
}

func validPackJSON(data []byte, label string) error {
	if err := uniqueJSON(data); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}
func decodeP6Manifest(data []byte, destination *p6Manifest, label string) error {
	if err := validPackJSON(data, label); err != nil {
		return err
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}
func decodeP8P10Manifest(data []byte, destination *p8P10Manifest, label string) error {
	if err := validPackJSON(data, label); err != nil {
		return err
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}
func decodeP8P10Golden(data []byte, destination *p8P10Golden, label string) error {
	if err := validPackJSON(data, label); err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}
func decodeP11P12Declarative(data []byte, destination *p11P12DeclarativeGolden, label string) error {
	if err := validPackJSON(data, label); err != nil {
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}
func jsonArrayEntry(fragment string, arrays map[string][]string) (string, error) {
	parts := strings.Split(strings.TrimPrefix(fragment, "/"), "/")
	if len(parts) != 2 || fragment[0] != '/' {
		return "", fmt.Errorf("invalid JSON pointer")
	}
	values, ok := arrays[parts[0]]
	if !ok {
		return "", fmt.Errorf("pointer does not name a manifest array")
	}
	index, err := jsonArrayIndex(fragment, parts[0], len(values))
	if err != nil {
		return "", err
	}
	return values[index], nil
}
func jsonArrayIndex(fragment, array string, length int) (int, error) {
	var index int
	if _, err := fmt.Sscanf(fragment, "/"+array+"/%d", &index); err != nil || fmt.Sprintf("/%s/%d", array, index) != fragment || index < 0 || index >= length {
		return 0, fmt.Errorf("pointer does not resolve to an array entry")
	}
	return index, nil
}
func markdownTableRow(data []byte, anchor string) ([]string, error) {
	id := strings.ToUpper(anchor)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || strings.Contains(line, "---") {
			continue
		}
		parts := strings.Split(strings.Trim(line, "|"), "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if len(parts) >= 2 && strings.EqualFold(parts[0], id) {
			return parts, nil
		}
	}
	return nil, fmt.Errorf("Markdown anchor does not resolve to a table row")
}
func markdownModeCapability(mode string) string {
	capability, ok := strings.CutPrefix(mode, "controlled:")
	if !ok {
		return ""
	}
	switch capability {
	case "fault_injection":
		return capability
	case "large_input":
		return "deterministic_blob_limit_input"
	case "process":
		return "process_orchestration"
	}
	return ""
}
func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func stringSubset(want, have []string) bool {
	for _, value := range want {
		if !containsString(have, value) {
			return false
		}
	}
	return true
}
func manifestCapabilitiesDeclared(want, have []string) bool {
	for _, value := range want {
		if containsString(have, value) {
			continue
		}
		// The identity-replacement adapter includes a held actor and the
		// scheduling boundary required by this manifest's process wording.
		if value == "process_orchestration" && containsString(have, "held_lock_identity_replacement") {
			continue
		}
		return false
	}
	return true
}

// Declarative P8/P10 goldens deliberately model only their published DSL. No
// map or RawMessage escape hatch is used: an added field must be reviewed and
// typed here, instead of allowing a syntactically valid but opaque `{}`.
type p8P10Golden struct {
	Case       string     `json:"case"`
	Setup      *p8Setup   `json:"setup"`
	Command    string     `json:"command"`
	Barrier    string     `json:"barrier"`
	Schedule   []string   `json:"schedule"`
	Matrix     []p8Matrix `json:"matrix"`
	Fault      *p8Fault   `json:"fault"`
	Actors     []string   `json:"actors"`
	Host       *p8Host    `json:"host"`
	Commands   []string   `json:"commands"`
	Operations []string   `json:"operations"`
	Expect     p8Expect   `json:"expect"`
}
type p8Setup struct {
	Target string `json:"target"`
	Peer   string `json:"peer"`
}
type p8Matrix struct {
	Text           string
	Lock           string `json:"lock"`
	Reason         string `json:"reason"`
	TargetMode     string `json:"target_mode"`
	ExpectMode     string `json:"expect_mode"`
	ExpectLockMode string `json:"expect_lock_mode"`
	ExpectReused   *bool  `json:"expect_reused"`
	Target         string `json:"target"`
	Umask          string `json:"umask"`
}

func (matrix *p8Matrix) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &matrix.Text)
	}
	type closedMatrix p8Matrix
	var decoded closedMatrix
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*matrix = p8Matrix(decoded)
	return nil
}

type p8Fault struct {
	Operation  string       `json:"operation"`
	When       string       `json:"when"`
	Outcome    string       `json:"outcome"`
	Outcomes   p8StringList `json:"outcomes"`
	Operations []string     `json:"operations"`
}
type p8BoolClaim struct {
	Bool *bool
	Text string
}

func (claim *p8BoolClaim) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &claim.Text)
	}
	return json.Unmarshal(data, &claim.Bool)
}

type p8StringList []string

func (values *p8StringList) UnmarshalJSON(data []byte) error {
	var one string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &one); err != nil {
			return err
		}
		*values = []string{one}
		return nil
	}
	return json.Unmarshal(data, (*[]string)(values))
}

type p8Host struct {
	OSArch string `json:"os_arch"`
}
type p8Expect struct {
	Status                       *int         `json:"status"`
	Error                        string       `json:"error"`
	Details                      p8Details    `json:"details"`
	Unchanged                    []string     `json:"unchanged"`
	Stdout                       string       `json:"stdout"`
	AtomicReplace                bool         `json:"atomic_replace"`
	SubstitutePreserved          *bool        `json:"substitute_preserved"`
	CommitMayHaveSucceeded       p8BoolClaim  `json:"commit_may_have_succeeded"`
	OldBytesPreserved            *bool        `json:"old_bytes_preserved"`
	DurableSuccess               *bool        `json:"durable_success"`
	Output                       string       `json:"output"`
	NoProtocolError              *bool        `json:"no_protocol_error"`
	ConcurrentCommit             *bool        `json:"concurrent_commit"`
	WaiterObservesHolderRevision *bool        `json:"waiter_observes_holder_revision"`
	AcceptedUpdates              *int         `json:"accepted_updates"`
	StaleResult                  *p8Conflict  `json:"stale_result"`
	ExpectLockMode               string       `json:"expect_lock_mode"`
	PersistentLock               *bool        `json:"persistent_lock"`
	TargetWrite                  string       `json:"target_write"`
	TempMode                     string       `json:"temp_mode"`
	TempSurvivors                *int         `json:"temp_survivors"`
	Successes                    *int         `json:"successes"`
	Conflicts                    []p8Conflict `json:"conflicts"`
	Final                        string       `json:"final"`
	OpenedTarget                 *bool        `json:"opened_target"`
}
type p8Details struct {
	Path                   string `json:"path"`
	Reason                 string `json:"reason"`
	Kind                   string `json:"kind"`
	Operation              string `json:"operation"`
	CommitMayHaveSucceeded *bool  `json:"commit_may_have_succeeded"`
}
type p8Conflict struct {
	Status int    `json:"status"`
	Kind   string `json:"kind"`
}

func validateP8P10Golden(data []byte, path, scenarioID string) error {
	var golden p8P10Golden
	if err := decodeP8P10Golden(data, &golden, path); err != nil {
		return err
	}
	if golden.Case == "" || golden.Case != scenarioID {
		return fmt.Errorf("%s: declarative case does not match scenario", path)
	}
	if golden.Expect.Status == nil && golden.Expect.Successes == nil && golden.Expect.PersistentLock == nil && golden.Expect.ConcurrentCommit == nil && golden.Expect.WaiterObservesHolderRevision == nil && golden.Expect.AcceptedUpdates == nil {
		return fmt.Errorf("%s: declarative expectation has no outcome", path)
	}
	return nil
}

type p11P12DeclarativeGolden struct {
	PackFixtureVersion int                  `json:"pack_fixture_version"`
	ID                 string               `json:"id"`
	Kind               string               `json:"kind"`
	Requires           []string             `json:"requires"`
	Argv               []string             `json:"argv"`
	OperationsRequest  p11OperationsRequest `json:"operations_request"`
	InitialTarget      string               `json:"initial_target"`
	Expect             p11Expect            `json:"expect"`
	UnavailableOutcome string               `json:"unavailable_outcome"`
}
type p11OperationsRequest struct {
	Protocol   int            `json:"protocol"`
	Operations []p11Operation `json:"operations"`
}
type p11Operation struct {
	Op        string       `json:"op"`
	ThreadID  string       `json:"thread_id"`
	MessageID string       `json:"message_id"`
	Location  *p11Location `json:"location"`
	Author    *p11Author   `json:"author"`
	Body      string       `json:"body"`
}
type p11Location struct {
	Kind string `json:"kind"`
}
type p11Author struct {
	Kind string `json:"kind"`
}
type p11Expect struct {
	Status                  *int      `json:"status"`
	Stdout                  string    `json:"stdout"`
	Stderr                  p11Stderr `json:"stderr"`
	Changed                 *bool     `json:"changed"`
	OrderedOperationResults []string  `json:"ordered_operation_results"`
	FinalTarget             string    `json:"final_target"`
	AtomicCommits           *int      `json:"atomic_commits"`
	PartialResults          *bool     `json:"partial_results"`
}
type p11Stderr struct {
	Text     string
	Envelope *p11ErrorEnvelope
}
type p11ErrorEnvelope struct {
	Protocol int      `json:"protocol"`
	Error    p11Error `json:"error"`
}
type p11Error struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Details p11ErrorDetails `json:"details"`
}
type p11ErrorDetails struct {
	Entity string `json:"entity"`
	ID     string `json:"id"`
	Index  *int   `json:"index"`
}

func (stderr *p11Stderr) UnmarshalJSON(data []byte) error {
	if len(data) > 0 && data[0] == '"' {
		return json.Unmarshal(data, &stderr.Text)
	}
	var envelope p11ErrorEnvelope
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		return err
	}
	stderr.Envelope = &envelope
	return nil
}
func validateP11P12DeclarativeGolden(data []byte, path, scenarioID string) error {
	var golden p11P12DeclarativeGolden
	if err := decodeP11P12Declarative(data, &golden, path); err != nil {
		return err
	}
	if golden.PackFixtureVersion != 1 || golden.ID == "" || golden.Kind != "controlled-golden" || golden.Expect.Status == nil || len(golden.Argv) == 0 || golden.OperationsRequest.Protocol != 1 || len(golden.OperationsRequest.Operations) == 0 {
		return fmt.Errorf("%s: incomplete P11/P12 declarative golden", path)
	}
	return nil
}

func requiredRedScenarioLines(base string) ([]string, error) {
	registry, err := validateRequiredRedRegistry(base)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(registry.Scenarios))
	for _, scenario := range registry.Scenarios {
		ref := scenario.ManifestRef
		if scenario.GoldenRef != "" {
			ref += " | " + scenario.GoldenRef
		}
		if len(scenario.Capabilities) != 0 {
			ref += " | requires " + strings.Join(scenario.Capabilities, ",")
		}
		out = append(out, scenario.ID+": REQUIRED_RED "+ref)
	}
	sort.Strings(out)
	return out, nil
}
