//go:build !(darwin && arm64) && !(linux && amd64)

package store

import (
	"io"
	"os"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type Transaction struct{}

func CheckWriterCapability(r CheckWriterCapabilityRequest) error {
	return domain.IO(domain.IORequest{Operation: "unsupported_secure_filesystem", Path: "target"})
}
func Begin(r BeginRequest) (*Transaction, error) {
	return nil, CheckWriterCapability(CheckWriterCapabilityRequest{})
}
func (t *Transaction) Read(r TransactionReadRequest) (Snapshot, error) {
	return Snapshot{}, domain.IO(domain.IORequest{Operation: "unsupported_secure_filesystem", Path: "target"})
}
func (t *Transaction) Commit(r CommitRequest) error {
	return domain.IO(domain.IORequest{Operation: "unsupported_secure_filesystem", Path: "target"})
}
func (t *Transaction) Close(r TransactionCloseRequest) error { return nil }
func Read(r ReadRequest) (s Snapshot, err error) {
	limit, e := readLimit(r)
	if e != nil {
		return s, e
	}
	kind := string(r.Kind)
	i, e := os.Lstat(r.Path)
	if os.IsNotExist(e) && r.Kind == ReadTarget {
		return s, nil
	}
	if e != nil {
		return s, domain.IO(domain.IORequest{Operation: "open", Path: kind})
	}
	reason := ""
	switch {
	case i.Mode()&os.ModeSymlink != 0:
		reason = "symlink"
	case i.IsDir():
		reason = "directory"
	case !i.Mode().IsRegular():
		reason = "nonregular"
	}
	if reason != "" {
		return s, validation(domain.ValidationRequest{Path: kind + "_type", Reason: reason})
	}
	if i.Size() > limit {
		return s, validation(domain.ValidationRequest{Path: kind + "_size", Reason: "too_large"})
	}
	f, e := os.Open(r.Path)
	if e != nil {
		return s, domain.IO(domain.IORequest{Operation: "open", Path: kind})
	}
	defer func(request cleanupRequest) {
		if e := f.Close(); e != nil && err == nil {
			err = domain.IO(domain.IORequest{Operation: "close", Path: kind})
		}
	}(cleanupRequest{})
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil {
		return s, domain.IO(domain.IORequest{Operation: "read", Path: kind})
	}
	if int64(len(b)) > limit {
		return s, validation(domain.ValidationRequest{Path: kind + "_size", Reason: "too_large"})
	}
	rev := domain.HashRevision(domain.HashRevisionRequest{Bytes: b})
	return Snapshot{Bytes: b, Exists: true, Revision: &rev}, nil
}
