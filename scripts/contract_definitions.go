// Command contract_definitions derives the public contract definition artifact
// from the domain's authoritative constants. The independent runner consumes
// the resulting JSON, never private production packages.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

const definitionArtifactPath = "contracts/schema/runtime-v2.json"

type definitionArtifact struct {
	Protocol       int            `json:"protocol"`
	Format         int            `json:"format"`
	Tracking       int            `json:"tracking"`
	Limits         map[string]int `json:"limits"`
	LocationKinds  []string       `json:"location_kinds"`
	AuthorKinds    []string       `json:"author_kinds"`
	OperationKinds []string       `json:"operation_kinds"`
}
type definitionsRequest struct{ Check, Write, Versions bool }

func definitions(r definitionsRequest) error {
	modes := 0
	for _, enabled := range []bool{r.Check, r.Write, r.Versions} {
		if enabled {
			modes++
		}
	}
	if modes != 1 {
		return errors.New("select exactly one of --check, --write or --versions")
	}
	if r.Versions {
		_, err := fmt.Fprintf(os.Stdout, "format=%d\nprotocol=%d\n", domain.FormatVersion, domain.ProtocolVersion)
		return err
	}
	artifact := definitionArtifact{
		Protocol: domain.ProtocolVersion, Format: domain.FormatVersion, Tracking: domain.TrackingVersion,
		Limits: map[string]int{
			"target_bytes": domain.MaxTargetBytes, "source_bytes": domain.MaxSourceBytes,
			"threads": domain.MaxThreads, "messages_thread": domain.MaxMessagesThread,
			"messages_document": domain.MaxMessagesDocument, "selection_bytes": domain.MaxAnchorBytes,
			"body_bytes": domain.MaxBodyBytes, "message_metadata_bytes": domain.MaxMetadataBytes,
			"apply_bytes": domain.MaxApplyBytes, "apply_operations": domain.MaxApplyOperations,
			"context_bytes": domain.MaxContextBytes, "min_match_context_bytes": domain.MinMatchContextBytes,
			"sync_work_bytes": domain.MaxSyncWorkBytes, "tracking_bytes": domain.MaxTrackingBytes,
			"tracking_metadata_bytes": domain.MaxTrackingMetadataBytes,
		},
		LocationKinds:  []string{domain.LocationFile, domain.LocationLine, domain.LocationRange, domain.LocationLost, domain.LocationUnresolved},
		AuthorKinds:    []string{domain.AuthorHuman, domain.AuthorAgent, domain.AuthorExternal},
		OperationKinds: []string{domain.OpThreadCreate, domain.OpThreadMove, domain.OpThreadDelete, domain.OpMessageAdd, domain.OpMessageUpdate, domain.OpMessageDelete},
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(artifact); err != nil {
		return fmt.Errorf("encode public definitions: %w", err)
	}
	if r.Write {
		return os.WriteFile(definitionArtifactPath, encoded.Bytes(), 0644)
	}
	observed, err := os.ReadFile(definitionArtifactPath)
	if err != nil {
		return fmt.Errorf("read public definitions: %w", err)
	}
	if !bytes.Equal(observed, encoded.Bytes()) {
		return fmt.Errorf("%s is stale; run go run scripts/contract_definitions.go --write", definitionArtifactPath)
	}
	return nil
}

func main() {
	check := flag.Bool("check", false, "Check the generated public definition artifact without writing.")
	write := flag.Bool("write", false, "Write the generated public definition artifact.")
	versions := flag.Bool("versions", false, "Print format/protocol fields for release build metadata.")
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "unexpected positional arguments")
		os.Exit(2)
	}
	if err := definitions(definitionsRequest{Check: *check, Write: *write, Versions: *versions}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
