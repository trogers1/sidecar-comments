package runner

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	corpusSchemaURL   = "https://github.com/trogers1/sidecar-comments/contracts/schema/corpus-v1.json"
	protocolSchemaURL = "https://github.com/trogers1/sidecar-comments/contracts/schema/v1.json"
	documentSchemaURL = "https://github.com/trogers1/sidecar-comments/docs/v1/schema/v1.json"
)

type schemaValidationRequest struct {
	Base         string
	Corpus       []byte
	ParsedCorpus corpus
	CorpusPath   string
}

// protocolValidator keeps the jsonschema implementation confined to this
// adapter while giving the runner a typed protocol-validation operation.
type protocolValidator struct {
	schema *jsonschema.Schema
}

type schemaResource struct {
	Location string
	Source   []byte
}

type schemaValidator struct {
	schema *jsonschema.Schema
}

// packGoldenValidators are compiled once for required-red registry validation.
// They apply the same corpus golden and protocol schemas used by the central
// corpus to any registered pack artifact that has the runner fixture shape.
type packGoldenValidators struct {
	golden   *jsonschema.Schema
	protocol *jsonschema.Schema
}

// validateJSONSchemas validates schema documents through their Draft 2020-12
// metaschema and validates every corpus instance through its declared schema.
// Typed decoding remains in decodeCorpus/decodeGolden; the schema check catches
// constraints which Go's structural decoder cannot express. The two helpers
// below are the sole jsonschema generic-instance bridge: they are harness-only,
// are reached only after concrete corpus/golden DTO decoding (or for observed
// child protocol bytes), and must not be copied into production code or used
// as a trust-boundary DTO decoder.
func validateJSONSchemas(request schemaValidationRequest) error {
	corpusSchema, err := readUnique(filepath.Join(request.Base, "contracts/schema/corpus-v1.json"))
	if err != nil {
		return err
	}
	protocolReference, err := readUnique(filepath.Join(request.Base, "contracts/schema/v1.json"))
	if err != nil {
		return err
	}
	documentSchema, err := readUnique(filepath.Join(request.Base, "docs/v1/schema/v1.json"))
	if err != nil {
		return err
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err = addSchemaResource(compiler, corpusSchemaURL, corpusSchema); err != nil {
		return fmt.Errorf("corpus schema: %w", err)
	}
	if err = addSchemaResource(compiler, protocolSchemaURL, protocolReference); err != nil {
		return fmt.Errorf("protocol reference schema: %w", err)
	}
	if err = addSchemaResource(compiler, documentSchemaURL, documentSchema); err != nil {
		return fmt.Errorf("normative protocol schema: %w", err)
	}
	corpusValidator, err := compiler.Compile(corpusSchemaURL)
	if err != nil {
		return fmt.Errorf("corpus schema meta-validation: %w", err)
	}
	protocolValidator, err := compiler.Compile(protocolSchemaURL)
	if err != nil {
		return fmt.Errorf("protocol schema meta-validation: %w", err)
	}
	if err = validateSchemaInstance(corpusValidator, request.Corpus); err != nil {
		return fmt.Errorf("%s schema validation: %w", request.CorpusPath, err)
	}
	goldenValidator, err := compiler.Compile(corpusSchemaURL + "#/$defs/golden")
	if err != nil {
		return fmt.Errorf("golden schema meta-validation: %w", err)
	}
	return validateGoldenSchemas(request.Base, goldenValidator, protocolValidator, request.ParsedCorpus)
}

// compileSchemaValidator is the typed test and harness entry point for schema
// compilation. Callers receive validation operations, never jsonschema types.
func compileSchemaValidator(location string, resources []schemaResource) (schemaValidator, error) {
	compiler := jsonschema.NewCompiler()
	for _, resource := range resources {
		if err := addSchemaResource(compiler, resource.Location, resource.Source); err != nil {
			return schemaValidator{}, err
		}
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		return schemaValidator{}, err
	}
	return schemaValidator{schema: schema}, nil
}

func (validator schemaValidator) validate(source []byte) error {
	return validateSchemaInstance(validator.schema, source)
}

func addSchemaResource(compiler *jsonschema.Compiler, location string, source []byte) error {
	var document interface{}
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	return compiler.AddResource(location, document)
}

func validateSchemaInstance(schema *jsonschema.Schema, source []byte) error {
	var instance interface{}
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	if err := decoder.Decode(&instance); err != nil {
		return err
	}
	return schema.Validate(instance)
}

func validateGoldenSchemas(base string, schema *jsonschema.Schema, protocol *jsonschema.Schema, manifest corpus) error {
	for _, fixture := range manifest.Fixtures {
		goldenPath := filepath.Join(base, fixture.Golden)
		goldenBytes, err := readUnique(goldenPath)
		if err != nil {
			return err
		}
		var fixtureDTO golden
		if err = decodeJSONGolden(goldenBytes, &fixtureDTO, fixture.Golden); err != nil {
			return err
		}
		// The corpus schema intentionally defines both the manifest and fixture
		// DTOs. A wrapper makes the golden branch reachable without weakening the
		// checked-in manifest's closed top-level shape.
		if err = validateSchemaInstance(schema, goldenBytes); err != nil {
			return fmt.Errorf("%s schema validation: %w", fixture.Golden, err)
		}
		if err = validateGoldenProtocolInstances(protocol, goldenBytes); err != nil {
			return fmt.Errorf("%s protocol instance validation: %w", fixture.Golden, err)
		}
	}
	return nil
}

// compilePackGoldenValidators compiles the central corpus-golden and protocol
// schemas for runner-shaped pack fixtures. Pack registration is artifact
// validation only; this does not execute or promote a pack scenario.
func compilePackGoldenValidators(base string) (*packGoldenValidators, error) {
	corpusSchema, err := readUnique(filepath.Join(base, "contracts/schema/corpus-v1.json"))
	if err != nil {
		return nil, err
	}
	protocolReference, err := readUnique(filepath.Join(base, "contracts/schema/v1.json"))
	if err != nil {
		return nil, err
	}
	documentSchema, err := readUnique(filepath.Join(base, "docs/v1/schema/v1.json"))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err = addSchemaResource(compiler, corpusSchemaURL, corpusSchema); err != nil {
		return nil, fmt.Errorf("corpus schema: %w", err)
	}
	if err = addSchemaResource(compiler, protocolSchemaURL, protocolReference); err != nil {
		return nil, fmt.Errorf("protocol reference schema: %w", err)
	}
	if err = addSchemaResource(compiler, documentSchemaURL, documentSchema); err != nil {
		return nil, fmt.Errorf("normative protocol schema: %w", err)
	}
	golden, err := compiler.Compile(corpusSchemaURL + "#/$defs/golden")
	if err != nil {
		return nil, fmt.Errorf("golden schema meta-validation: %w", err)
	}
	protocol, err := compiler.Compile(protocolSchemaURL)
	if err != nil {
		return nil, fmt.Errorf("protocol schema meta-validation: %w", err)
	}
	return &packGoldenValidators{golden: golden, protocol: protocol}, nil
}

// validateRunnerPackGolden gives registered runner-shaped packs the same
// schema and expected-protocol-instance checks as central corpus goldens.
func validateRunnerPackGolden(path string, data []byte, validators *packGoldenValidators) (golden, error) {
	var fixture golden
	if err := decodeJSONGolden(data, &fixture, path); err != nil {
		return fixture, err
	}
	if err := validateSchemaInstance(validators.golden, data); err != nil {
		return fixture, fmt.Errorf("%s schema validation: %w", path, err)
	}
	if err := validateRunnerPackGoldenProtocolInstances(validators.protocol, data); err != nil {
		return fixture, fmt.Errorf("%s protocol instance validation: %w", path, err)
	}
	return fixture, nil
}

// validateRunnerPackGoldenProtocolInstances validates every JSON protocol
// stream in runner-shaped pack fixtures against the normative protocol schema.
func validateRunnerPackGoldenProtocolInstances(protocol *jsonschema.Schema, source []byte) error {
	var fixture golden
	if err := decodeJSONGolden(source, &fixture, "golden"); err != nil {
		return err
	}
	stdout, err := expectedBytes(expectedBytesRequest{
		Encoded:   fixture.Expect.Stdout,
		Template:  fixture.Expect.StdoutTemplate,
		Expansion: expandRequest{Initial: fixture.Initial, Final: fixture.Expect.Final, CLIVersion: "schema"},
	})
	if err != nil {
		return err
	}
	stderr, err := expectedBytes(expectedBytesRequest{
		Encoded:   fixture.Expect.Stderr,
		Template:  fixture.Expect.StderrTemplate,
		Expansion: expandRequest{Initial: fixture.Initial, Final: fixture.Expect.Final, CLIVersion: "schema"},
	})
	if err != nil {
		return err
	}
	if err = validateEnvelopes(fixture, *fixture.Expect.Status, stdout, stderr); err != nil {
		return fmt.Errorf("fixture stream: %w", err)
	}
	if !jsonOutput(fixture.Argv) {
		return nil
	}
	if len(stdout) != 0 {
		if err := validateActualProtocol(protocol, stdout, nil); err != nil {
			return fmt.Errorf("stdout: %w", err)
		}
	}
	if len(stderr) != 0 {
		if err := validateActualProtocol(protocol, nil, stderr); err != nil {
			return fmt.Errorf("stderr: %w", err)
		}
	}
	return nil
}

func compileProtocolValidator(base string) (*protocolValidator, error) {
	protocolReference, err := readUnique(filepath.Join(base, "contracts/schema/v1.json"))
	if err != nil {
		return nil, err
	}
	documentSchema, err := readUnique(filepath.Join(base, "docs/v1/schema/v1.json"))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err = addSchemaResource(compiler, protocolSchemaURL, protocolReference); err != nil {
		return nil, err
	}
	if err = addSchemaResource(compiler, documentSchemaURL, documentSchema); err != nil {
		return nil, err
	}
	validator, err := compiler.Compile(protocolSchemaURL)
	if err != nil {
		return nil, fmt.Errorf("protocol schema meta-validation: %w", err)
	}
	return &protocolValidator{schema: validator}, nil
}

func (validator *protocolValidator) validateActual(stdout, stderr []byte) error {
	return validateActualProtocol(validator.schema, stdout, stderr)
}

// validateActualProtocol validates bytes produced by the child, after stream
// direction and envelope-discriminator checks. It deliberately validates the
// observed child output, not only the checked-in expected bytes.
func validateActualProtocol(protocol *jsonschema.Schema, stdout, stderr []byte) error {
	stream := stdout
	if len(stream) == 0 {
		stream = stderr
	}
	if len(stream) == 0 {
		return errors.New("missing JSON protocol stream")
	}
	instance := stream[:len(stream)-1]
	if err := validateSchemaInstance(protocol, instance); err != nil {
		return fmt.Errorf("child protocol schema validation: %w", err)
	}
	return nil
}

func validateGoldenProtocolInstances(protocol *jsonschema.Schema, source []byte) error {
	var fixture golden
	if err := decodeJSONGolden(source, &fixture, "golden"); err != nil {
		return err
	}
	stdout, err := expectedBytes(expectedBytesRequest{
		Encoded:   fixture.Expect.Stdout,
		Template:  fixture.Expect.StdoutTemplate,
		Expansion: expandRequest{Initial: fixture.Initial, Final: fixture.Expect.Final, CLIVersion: "schema"},
	})
	if err != nil {
		return err
	}
	stderr, err := expectedBytes(expectedBytesRequest{
		Encoded:   fixture.Expect.Stderr,
		Template:  fixture.Expect.StderrTemplate,
		Expansion: expandRequest{Initial: fixture.Initial, Final: fixture.Expect.Final, CLIVersion: "schema"},
	})
	if err != nil {
		return err
	}
	if err = validateEnvelopes(fixture, *fixture.Expect.Status, stdout, stderr); err != nil {
		return fmt.Errorf("fixture stream: %w", err)
	}
	if jsonOutput(fixture.Argv) {
		return validateActualProtocol(protocol, stdout, stderr)
	}
	invocation, invocationErr := parseInvocation(fixture.Argv)
	if invocationErr != nil || invocation.output != "text" || len(stdout) == 0 {
		return nil
	}
	textEnvelope, textErr := textSuccessEnvelope(stdout, invocation.command)
	if textErr != nil {
		return textErr
	}
	return validateSchemaInstance(protocol, textEnvelope)
}
