package runner

// This file is deliberately a decoder and validator for the v2 *design*
// artifacts. It has no process, filesystem, editor, platform, or fault
// control implementation. In particular, a decoded `proven` declaration is
// evidence-shaped metadata, not an execution claim by this package.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const capabilityV2Schema = "contracts/schema/corpus-v2.json"
const capabilityV2SchemaURL = "https://github.com/trogers1/sidecar-comments/contracts/schema/corpus-v2.json"

// ValidateV2CapabilityDesign checks the shadow catalog and schema without
// treating any declared capability as available or any fixture as proven.
func ValidateV2CapabilityDesign(base string) error {
	catalog, err := readUnique(filepath.Join(base, "contracts/packs/capabilities-v2.json"))
	if err != nil {
		return err
	}
	schema, err := os.ReadFile(filepath.Join(base, capabilityV2Schema))
	if err != nil {
		return err
	}
	validator, err := compileSchemaValidator(capabilityV2SchemaURL, []schemaResource{{Location: capabilityV2SchemaURL, Source: schema}})
	if err != nil {
		return err
	}
	if err := validator.validate(catalog); err != nil {
		return fmt.Errorf("v2 capability catalog schema: %w", err)
	}
	_, err = decodeCapabilityCatalogV2(catalog)
	return err
}

type capabilityCatalogV2 struct {
	CapabilityVersion  int                  `json:"capability_version"`
	Schema             string               `json:"schema"`
	DesignState        string               `json:"design_state"`
	UnavailableOutcome unavailableOutcomeV2 `json:"unavailable_outcome"`
	Capabilities       []capabilitySpecV2   `json:"capabilities"`
}
type unavailableOutcomeV2 struct {
	Result       string   `json:"result"`
	FailureClass string   `json:"failure_class"`
	Nonzero      bool     `json:"nonzero"`
	Never        []string `json:"never"`
}
type capabilitySpecV2 struct {
	ID                string             `json:"id"`
	InputKind         string             `json:"input_kind"`
	Status            string             `json:"status"`
	UnavailableReason string             `json:"unavailable_reason"`
	Proof             *capabilityProofV2 `json:"proof"`
	Guarantees        []string           `json:"guarantees"`
}
type capabilityProofV2 struct {
	Method   string `json:"method"`
	Artifact string `json:"artifact"`
}

type controlledFixtureV2 struct {
	FixtureVersion int               `json:"fixture_version"`
	ID             string            `json:"id"`
	Capability     string            `json:"capability"`
	Input          controlledInputV2 `json:"input"`
}

type controlledInputV2 interface{ inputKindV2() string }
type deterministicBlobInputV2 struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Algorithm  string `json:"algorithm"`
	SeedB64    string `json:"seed_b64"`
	ByteLength int    `json:"byte_length"`
	Limit      int    `json:"limit"`
}

func (deterministicBlobInputV2) inputKindV2() string { return "deterministic_blob" }

type actorV2 struct {
	ID             string   `json:"id"`
	Argv           []string `json:"argv"`
	ReadyBarrier   string   `json:"ready_barrier"`
	ReleaseBarrier string   `json:"release_barrier"`
}
type actorsBarriersInputV2 struct {
	Kind         string    `json:"kind"`
	Actors       []actorV2 `json:"actors"`
	Barriers     []string  `json:"barriers"`
	ReleaseOrder []string  `json:"release_order"`
	TimeoutMS    int       `json:"timeout_ms"`
}

func (actorsBarriersInputV2) inputKindV2() string { return "actors_barriers" }

type replacementV2 struct {
	Kind       string `json:"kind"`
	ObjectKind string `json:"object_kind"`
}
type lockIdentityReplacementInputV2 struct {
	Kind        string        `json:"kind"`
	LockPath    string        `json:"lock_path"`
	HolderActor string        `json:"holder_actor"`
	AtBarrier   string        `json:"at_barrier"`
	Replacement replacementV2 `json:"replacement"`
}

func (lockIdentityReplacementInputV2) inputKindV2() string { return "lock_identity_replacement" }

type faultOutcomeV2 struct {
	Kind  string `json:"kind"`
	Errno string `json:"errno"`
	Bytes *int   `json:"bytes"`
}
type faultInjectionInputV2 struct {
	Kind       string         `json:"kind"`
	Scope      string         `json:"scope"`
	Operation  string         `json:"operation"`
	Occurrence int            `json:"occurrence"`
	Outcome    faultOutcomeV2 `json:"outcome"`
}

func (faultInjectionInputV2) inputKindV2() string { return "fault_injection" }

type platformV2 struct {
	GOOS   string `json:"goos"`
	GOARCH string `json:"goarch"`
}
type platformObjectUmaskInputV2 struct {
	Kind       string     `json:"kind"`
	Platform   platformV2 `json:"platform"`
	ObjectKind string     `json:"object_kind"`
	Umask      string     `json:"umask"`
}

func (platformObjectUmaskInputV2) inputKindV2() string { return "platform_object_umask" }

type editorInputV2 struct {
	Kind       string   `json:"kind"`
	Argv       []string `json:"argv"`
	Protocol   string   `json:"protocol"`
	ExitStatus int      `json:"exit_status"`
}

func (editorInputV2) inputKindV2() string { return "editor" }

func decodeCapabilityCatalogV2(data []byte) (capabilityCatalogV2, error) {
	var catalog capabilityCatalogV2
	decoder, err := newV2Decoder(data)
	if err != nil {
		return catalog, err
	}
	if err = decoder.Decode(&catalog); err != nil {
		return catalog, err
	}
	if err = finishV2Decode(decoder); err != nil {
		return catalog, err
	}
	return catalog, validateCapabilityCatalogV2(catalog)
}
func decodeControlledFixtureV2(data []byte) (controlledFixtureV2, error) {
	var fixture controlledFixtureV2
	var wire struct {
		FixtureVersion int             `json:"fixture_version"`
		ID             string          `json:"id"`
		Capability     string          `json:"capability"`
		Input          json.RawMessage `json:"input"`
	}
	decoder, err := newV2Decoder(data)
	if err != nil {
		return fixture, err
	}
	if err = decoder.Decode(&wire); err != nil {
		return fixture, err
	}
	if err = finishV2Decode(decoder); err != nil {
		return fixture, err
	}
	input, err := decodeControlledInputV2(wire.Input)
	if err != nil {
		return fixture, err
	}
	fixture = controlledFixtureV2{FixtureVersion: wire.FixtureVersion, ID: wire.ID, Capability: wire.Capability, Input: input}
	if fixture.FixtureVersion != 2 || fixture.ID == "" || fixture.Capability == "" {
		return fixture, fmt.Errorf("invalid v2 fixture header")
	}
	return fixture, nil
}
func decodeControlledInputV2(data []byte) (controlledInputV2, error) {
	var discriminator struct {
		Kind string `json:"kind"`
	}
	// Decode the discriminator without rejecting the fields owned by the
	// selected concrete DTO. Duplicate members remain rejected before either
	// decode; the selected decoder below rejects unknown fields.
	if err := uniqueJSON(data); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&discriminator); err != nil {
		return nil, err
	}
	if err := finishV2Decode(decoder); err != nil {
		return nil, err
	}
	var err error
	switch discriminator.Kind {
	case "deterministic_blob":
		var value deterministicBlobInputV2
		err = decodeV2DeterministicBlob(data, &value)
		return &value, err
	case "actors_barriers":
		var value actorsBarriersInputV2
		err = decodeV2ActorsBarriers(data, &value)
		return &value, err
	case "lock_identity_replacement":
		var value lockIdentityReplacementInputV2
		err = decodeV2LockReplacement(data, &value)
		return &value, err
	case "fault_injection":
		var value faultInjectionInputV2
		err = decodeV2FaultInjection(data, &value)
		return &value, err
	case "platform_object_umask":
		var value platformObjectUmaskInputV2
		err = decodeV2PlatformObjectUmask(data, &value)
		return &value, err
	case "editor":
		var value editorInputV2
		err = decodeV2Editor(data, &value)
		return &value, err
	default:
		return nil, fmt.Errorf("unknown v2 controlled input kind %q", discriminator.Kind)
	}
}
func newV2Decoder(data []byte) (*json.Decoder, error) {
	if err := uniqueJSON(data); err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	return decoder, nil
}
func finishV2Decode(decoder *json.Decoder) error {
	if decoder.Decode(&struct{}{}) != io.EOF {
		return fmt.Errorf("trailing JSON")
	}
	return nil
}
func decodeV2DeterministicBlob(data []byte, value *deterministicBlobInputV2) error {
	decoder, err := newV2Decoder(data)
	if err != nil {
		return err
	}
	if err = decoder.Decode(value); err != nil {
		return err
	}
	if err = finishV2Decode(decoder); err != nil {
		return err
	}
	return validateControlledInputV2(value)
}
func decodeV2ActorsBarriers(data []byte, value *actorsBarriersInputV2) error {
	decoder, err := newV2Decoder(data)
	if err != nil {
		return err
	}
	if err = decoder.Decode(value); err != nil {
		return err
	}
	if err = finishV2Decode(decoder); err != nil {
		return err
	}
	return validateControlledInputV2(value)
}
func decodeV2LockReplacement(data []byte, value *lockIdentityReplacementInputV2) error {
	decoder, err := newV2Decoder(data)
	if err != nil {
		return err
	}
	if err = decoder.Decode(value); err != nil {
		return err
	}
	if err = finishV2Decode(decoder); err != nil {
		return err
	}
	return validateControlledInputV2(value)
}
func decodeV2FaultInjection(data []byte, value *faultInjectionInputV2) error {
	decoder, err := newV2Decoder(data)
	if err != nil {
		return err
	}
	if err = decoder.Decode(value); err != nil {
		return err
	}
	if err = finishV2Decode(decoder); err != nil {
		return err
	}
	return validateControlledInputV2(value)
}
func decodeV2PlatformObjectUmask(data []byte, value *platformObjectUmaskInputV2) error {
	decoder, err := newV2Decoder(data)
	if err != nil {
		return err
	}
	if err = decoder.Decode(value); err != nil {
		return err
	}
	if err = finishV2Decode(decoder); err != nil {
		return err
	}
	return validateControlledInputV2(value)
}
func decodeV2Editor(data []byte, value *editorInputV2) error {
	decoder, err := newV2Decoder(data)
	if err != nil {
		return err
	}
	if err = decoder.Decode(value); err != nil {
		return err
	}
	if err = finishV2Decode(decoder); err != nil {
		return err
	}
	return validateControlledInputV2(value)
}

func validateCapabilityCatalogV2(c capabilityCatalogV2) error {
	if c.CapabilityVersion != 2 || c.Schema != capabilityV2Schema || c.DesignState != "declarative-only" || c.UnavailableOutcome.Result != "UNAVAILABLE" || c.UnavailableOutcome.FailureClass != "capability" || !c.UnavailableOutcome.Nonzero || !sameStrings(c.UnavailableOutcome.Never, []string{"skip", "pass", "proven"}) {
		return fmt.Errorf("invalid v2 capability catalog header")
	}
	seenID, seenKind := map[string]bool{}, map[string]bool{}
	for _, spec := range c.Capabilities {
		if spec.ID == "" || !knownV2InputKind(spec.InputKind) || seenID[spec.ID] || seenKind[spec.InputKind] || len(spec.Guarantees) == 0 {
			return fmt.Errorf("invalid v2 capability %q", spec.ID)
		}
		seenID[spec.ID], seenKind[spec.InputKind] = true, true
		switch spec.Status {
		case "unavailable":
			if spec.UnavailableReason == "" || spec.Proof != nil {
				return fmt.Errorf("invalid unavailable v2 capability %q", spec.ID)
			}
		case "proven":
			// This catalog is a declarative design. Proof-shaped metadata must
			// not turn an unimplemented adapter into an available capability.
			return fmt.Errorf("v2 capability %q cannot be proven in declarative-only mode", spec.ID)
		default:
			return fmt.Errorf("unknown v2 capability status %q", spec.Status)
		}
	}
	if len(seenKind) != len(v2InputKinds) {
		return fmt.Errorf("v2 capability catalog does not cover typed dispatch")
	}
	return nil
}

var v2InputKinds = []string{"deterministic_blob", "actors_barriers", "lock_identity_replacement", "fault_injection", "platform_object_umask", "editor"}

func knownV2InputKind(kind string) bool {
	for _, known := range v2InputKinds {
		if kind == known {
			return true
		}
	}
	return false
}
func validateControlledInputV2(input controlledInputV2) error {
	if input == nil || !knownV2InputKind(input.inputKindV2()) {
		return fmt.Errorf("unknown v2 controlled input")
	}
	switch value := input.(type) {
	case *deterministicBlobInputV2:
		if value.Algorithm != "sha256-counter-v1" || value.Name == "" || value.ByteLength < 0 || value.Limit < 0 {
			return fmt.Errorf("invalid deterministic_blob input")
		}
	case *actorsBarriersInputV2:
		if len(value.Actors) < 2 || len(value.Barriers) == 0 || len(value.ReleaseOrder) == 0 || value.TimeoutMS < 1 {
			return fmt.Errorf("invalid actors_barriers input")
		}
	case *lockIdentityReplacementInputV2:
		if value.LockPath == "" || value.HolderActor == "" || value.AtBarrier == "" || (value.Replacement.Kind != "replace_target" && value.Replacement.Kind != "replace_lock") {
			return fmt.Errorf("invalid lock_identity_replacement input")
		}
	case *faultInjectionInputV2:
		if value.Occurrence < 1 || (value.Outcome.Kind != "errno" && value.Outcome.Kind != "short_write") {
			return fmt.Errorf("invalid fault_injection input")
		}
	case *platformObjectUmaskInputV2:
		if value.Platform.GOOS == "" || value.Platform.GOARCH == "" || len(value.Umask) != 4 {
			return fmt.Errorf("invalid platform_object_umask input")
		}
	case *editorInputV2:
		if len(value.Argv) == 0 || value.Protocol != "file-argument-v1" || value.ExitStatus < 0 || value.ExitStatus > 255 {
			return fmt.Errorf("invalid editor input")
		}
	default:
		return fmt.Errorf("unregistered v2 controlled input dispatch %T", input)
	}
	return nil
}
