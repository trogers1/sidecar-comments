// Package store owns bounded filesystem reads and locked, durable transactions.
package store

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type ReadKind string

const (
	ReadTarget ReadKind = "target"
	ReadSource ReadKind = "source"
)

type ReadRequest struct {
	Path string
	Kind ReadKind
}
type TransactionReadRequest struct{}
type TransactionCloseRequest struct{}

func readLimit(r ReadRequest) (int64, error) {
	switch r.Kind {
	case ReadTarget:
		if err := validateTargetPath(pathRequest{Path: r.Path}); err != nil {
			return 0, err
		}
		return domain.MaxTargetBytes, nil
	case ReadSource:
		return domain.MaxSourceBytes, nil
	default:
		return 0, fmt.Errorf("store: invalid read kind %q", r.Kind)
	}
}

type Snapshot struct {
	Bytes    []byte
	Exists   bool
	Revision *string
}

// StorageOperation identifies a transaction syscall boundary for fault tests.
// Close callbacks run after the real descriptor has been closed.
type StorageOperation struct{ Operation, Path string }

type BeginRequest struct {
	Path    string
	Timeout time.Duration
	// Fault is optional and normally nil. It injects an error at a transaction
	// syscall boundary without replacing the real transaction or filesystem.
	Fault func(StorageOperation) error
}
type CommitRequest struct {
	Bytes []byte
	// Validate runs immediately before destructive replacement/removal.
	Validate func(CommitValidationRequest) error
}
type CommitValidationRequest struct{}
type CheckWriterCapabilityRequest struct{}
type cleanupRequest struct{}
type pathRequest struct{ Path string }
type ReadSourceValueRequest struct{ Path string }
type ValidateSourceRequest struct {
	Path      string
	Locations []domain.Location
}

func validation(r domain.ValidationRequest) error { return domain.Validation(r) }

// ReadSourceValue uses the bounded, secure source reader before constructing
// the shared logical-source representation.
func ReadSourceValue(r ReadSourceValueRequest) (domain.SourceValue, error) {
	s, err := Read(ReadRequest{Path: r.Path, Kind: ReadSource})
	if err != nil {
		return domain.SourceValue{}, err
	}
	return domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: s.Bytes})
}

func ValidateSource(r ValidateSourceRequest) error {
	required := len(r.Locations) == 0
	for _, loc := range r.Locations {
		if loc.Kind != domain.LocationLost {
			required = true
		}
	}
	if !required {
		return nil
	}
	s, err := ReadSourceValue(ReadSourceValueRequest{Path: r.Path})
	if err != nil {
		return err
	}
	for _, loc := range r.Locations {
		if loc.Kind == domain.LocationUnresolved {
			continue
		}
		if err := s.ValidateLocation(domain.SourceLocationRequest{Location: loc}); err != nil {
			return err
		}
	}
	return nil
}

func validateTargetPath(r pathRequest) error {
	name := filepath.Base(r.Path)
	if r.Path == "" || strings.HasSuffix(r.Path, string(filepath.Separator)) || name == ".comments" || !strings.HasSuffix(name, ".comments") {
		return validation(domain.ValidationRequest{Path: "target_name", Reason: "invalid"})
	}
	return nil
}
