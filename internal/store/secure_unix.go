//go:build (darwin && arm64) || (linux && amd64)

package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type Transaction struct {
	root                       *os.Root
	parentPath, name, lockName string
	parent, lock, target       os.FileInfo
	lockFile                   *os.File
	durable, closed, read      bool
	// Tests may inject individual syscall failures on a private transaction.
	// Production transactions leave this nil; there is no external switch.
	fault func(storageOperation) error
}

type storageOperation = StorageOperation

type storageAction struct {
	storageOperation
	Action func() error
}

type fileClose struct {
	storageOperation
	File *os.File
}

func (t *Transaction) perform(r storageAction) error {
	if t.fault != nil {
		if err := t.fault(r.storageOperation); err != nil {
			return err
		}
	}
	return r.Action()
}

// Close faults are reported only after closing the real descriptor.
func (t *Transaction) closeFile(r fileClose) error {
	err := r.File.Close()
	if err != nil {
		return err
	}
	return t.perform(storageAction{storageOperation: r.storageOperation, Action: func() error { return nil }})
}

func CheckWriterCapability(r CheckWriterCapabilityRequest) error { return nil }

func ioe(r domain.IORequest) error { return domain.IO(r) }

type conflictRequest struct{ Kind string }
type checkLockRequest struct{}
type identitiesRequest struct{}
type uncertainRequest struct{}
type classifyRequest struct {
	Root       *os.Root
	Name, Kind string
	Flags      int
	Mode       os.FileMode
}
type readAtRequest struct {
	Root *os.Root
	Name string
	Kind ReadKind
}

func conflict(r conflictRequest) error {
	return domain.Conflict(domain.ConflictRequest{Kind: r.Kind})
}

// OpenRoot pins the directory descriptor, including when ancestors use normal
// platform aliases such as Darwin's /var. All child IO is relative to this
// descriptor, with no-follow opens; commit separately rechecks parent identity.
func openParent(r pathRequest) (*os.Root, error) {
	root, e := os.OpenRoot(r.Path)
	if e != nil {
		return nil, ioe(domain.IORequest{Operation: "open", Path: "parent"})
	}
	return root, nil
}
func classify(r classifyRequest) (*os.File, error) {
	path := r.Kind + "_type"
	if r.Kind == "lock" {
		path = "lock_type"
	}
	info, e := r.Root.Lstat(r.Name)
	if e == nil {
		reason := ""
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			reason = "symlink"
		case info.IsDir():
			reason = "directory"
		case !info.Mode().IsRegular():
			reason = "nonregular"
		}
		if reason != "" {
			return nil, validation(domain.ValidationRequest{Path: path, Reason: reason})
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return nil, ioe(domain.IORequest{Operation: "open", Path: r.Kind})
	}
	f, e := r.Root.OpenFile(r.Name, r.Flags|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, r.Mode)
	if e != nil {
		if errors.Is(e, syscall.ELOOP) {
			return nil, validation(domain.ValidationRequest{Path: path, Reason: "symlink"})
		}
		return nil, e
	}
	var st syscall.Stat_t
	if e = syscall.Fstat(int(f.Fd()), &st); e != nil {
		identityErr := ioe(domain.IORequest{Operation: "identity", Path: r.Kind})
		if closeErr := f.Close(); closeErr != nil {
			return nil, errors.Join(identityErr, ioe(domain.IORequest{Operation: "close", Path: r.Kind}))
		}
		return nil, identityErr
	}
	reason := ""
	switch {
	case st.Mode&syscall.S_IFMT == syscall.S_IFDIR:
		reason = "directory"
	case st.Mode&syscall.S_IFMT != syscall.S_IFREG:
		reason = "nonregular"
	case r.Kind != "source" && st.Nlink != 1:
		reason = "multilink"
	}
	if reason != "" {
		if closeErr := f.Close(); closeErr != nil {
			return nil, ioe(domain.IORequest{Operation: "close", Path: r.Kind})
		}
		return nil, validation(domain.ValidationRequest{Path: path, Reason: reason})
	}
	return f, nil
}
func readAt(r readAtRequest) (s Snapshot, info os.FileInfo, err error) {
	limit, e := readLimit(ReadRequest{Path: r.Name, Kind: r.Kind})
	if e != nil {
		return s, nil, e
	}
	kind := string(r.Kind)
	f, e := classify(classifyRequest{Root: r.Root, Name: r.Name, Kind: kind, Flags: os.O_RDONLY, Mode: 0})
	if e != nil {
		if errors.Is(e, os.ErrNotExist) && r.Kind == ReadTarget {
			return Snapshot{}, nil, nil
		}
		var de *domain.Error
		if errors.As(e, &de) {
			return s, nil, e
		}
		return s, nil, ioe(domain.IORequest{Operation: "open", Path: kind})
	}
	defer func(request cleanupRequest) {
		if e := f.Close(); e != nil && err == nil {
			err = ioe(domain.IORequest{Operation: "close", Path: kind})
		}
	}(cleanupRequest{})
	info, e = f.Stat()
	if e != nil {
		return s, nil, ioe(domain.IORequest{Operation: "identity", Path: kind})
	}
	if info.Size() > limit {
		return s, nil, validation(domain.ValidationRequest{Path: kind + "_size", Reason: "too_large"})
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil {
		return s, nil, ioe(domain.IORequest{Operation: "read", Path: kind})
	}
	if int64(len(b)) > limit {
		return s, nil, validation(domain.ValidationRequest{Path: kind + "_size", Reason: "too_large"})
	}
	rev := domain.HashRevision(domain.HashRevisionRequest{Bytes: b})
	return Snapshot{Bytes: b, Exists: true, Revision: &rev}, info, nil
}
func Read(r ReadRequest) (s Snapshot, err error) {
	if _, e := readLimit(r); e != nil {
		return s, e
	}
	root, e := openParent(pathRequest{Path: filepath.Dir(r.Path)})
	if e != nil {
		if r.Kind == ReadSource {
			return s, ioe(domain.IORequest{Operation: "open", Path: "source"})
		}
		return s, e
	}
	defer func(request cleanupRequest) {
		if e := root.Close(); e != nil && err == nil {
			err = ioe(domain.IORequest{Operation: "close", Path: "parent"})
		}
	}(cleanupRequest{})
	s, _, err = readAt(readAtRequest{Root: root, Name: filepath.Base(r.Path), Kind: r.Kind})
	return
}
func Begin(r BeginRequest) (*Transaction, error) {
	return begin(beginRequest{BeginRequest: r})
}

type beginRequest struct {
	BeginRequest
	// A private test barrier, called only after flock observes contention.
	waiting func()
}

func begin(r beginRequest) (tx *Transaction, err error) {
	if e := CheckWriterCapability(CheckWriterCapabilityRequest{}); e != nil {
		return nil, e
	}
	if e := validateTargetPath(pathRequest{Path: r.Path}); e != nil {
		return nil, e
	}
	root, e := openParent(pathRequest{Path: filepath.Dir(r.Path)})
	if e != nil {
		return nil, e
	}
	t := &Transaction{root: root, parentPath: filepath.Dir(r.Path), name: filepath.Base(r.Path), fault: r.Fault}
	t.lockName = "." + t.name + ".sidecar.lock"
	defer func(request cleanupRequest) {
		if err != nil {
			if ce := t.Close(TransactionCloseRequest{}); ce != nil {
				err = ce
			}
		}
	}(cleanupRequest{})
	t.parent, e = root.Stat(".")
	if e != nil {
		return nil, ioe(domain.IORequest{Operation: "identity", Path: "parent"})
	}
	// Probe directory durability before any target mutation; unsupported local
	// filesystem capabilities must never degrade into an unprotected write.
	dir, probeErr := root.Open(".")
	if probeErr != nil {
		return nil, ioe(domain.IORequest{Operation: "unsupported_secure_filesystem", Path: "target"})
	}
	probeErr = dir.Sync()
	closeErr := dir.Close()
	if probeErr != nil {
		return nil, ioe(domain.IORequest{Operation: "unsupported_secure_filesystem", Path: "target"})
	}
	if closeErr != nil {
		return nil, ioe(domain.IORequest{Operation: "close", Path: "parent"})
	}
	// Classify and bound the target before lock setup, as required by precedence.
	if _, _, e = readAt(readAtRequest{Root: root, Name: t.name, Kind: ReadTarget}); e != nil {
		return nil, e
	}
	t.lockFile, e = classify(classifyRequest{Root: root, Name: t.lockName, Kind: "lock", Flags: os.O_RDWR | os.O_CREATE, Mode: 0600})
	if e != nil {
		var de *domain.Error
		if errors.As(e, &de) {
			return nil, e
		}
		return nil, ioe(domain.IORequest{Operation: "open", Path: "lock"})
	}
	t.lock, e = t.lockFile.Stat()
	if e != nil {
		return nil, ioe(domain.IORequest{Operation: "identity", Path: "lock"})
	}
	if e = t.checkLock(checkLockRequest{}); e != nil {
		return nil, e
	}
	deadline := time.Now().Add(r.Timeout)
	for {
		e = syscall.Flock(int(t.lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if e == nil {
			break
		}
		if e != syscall.EWOULDBLOCK && e != syscall.EAGAIN && e != syscall.EINTR {
			return nil, ioe(domain.IORequest{Operation: "lock", Path: "lock"})
		}
		if r.waiting != nil {
			r.waiting()
			r.waiting = nil
		}
		if !time.Now().Before(deadline) {
			return nil, conflict(conflictRequest{Kind: "lock_timeout"})
		}
		time.Sleep(min(5*time.Millisecond, time.Until(deadline)))
	}
	if e = t.checkLock(checkLockRequest{}); e != nil {
		return nil, e
	}
	return t, nil
}
func (t *Transaction) checkLock(r checkLockRequest) error {
	i, e := t.root.Lstat(t.lockName)
	if e != nil {
		return ioe(domain.IORequest{Operation: "identity", Path: "lock"})
	}
	if !os.SameFile(t.lock, i) {
		return conflict(conflictRequest{Kind: "identity_changed"})
	}
	var st syscall.Stat_t
	if e := syscall.Fstat(int(t.lockFile.Fd()), &st); e != nil {
		return ioe(domain.IORequest{Operation: "identity", Path: "lock"})
	}
	if st.Nlink != 1 {
		return validation(domain.ValidationRequest{Path: "lock_type", Reason: "multilink"})
	}
	return nil
}
func (t *Transaction) Read(r TransactionReadRequest) (Snapshot, error) {
	if t.closed {
		return Snapshot{}, ioe(domain.IORequest{Operation: "read", Path: "target"})
	}
	s, i, e := readAt(readAtRequest{Root: t.root, Name: t.name, Kind: ReadTarget})
	if e == nil {
		t.target = i
		t.read = true
	}
	return s, e
}
func (t *Transaction) identities(r identitiesRequest) error {
	if e := t.checkLock(checkLockRequest{}); e != nil {
		return e
	}
	fresh, e := openParent(pathRequest{Path: t.parentPath})
	if e != nil {
		return e
	}
	i, e := fresh.Stat(".")
	ce := fresh.Close()
	if e != nil {
		identityErr := ioe(domain.IORequest{Operation: "identity", Path: "parent"})
		if ce != nil {
			return errors.Join(identityErr, ioe(domain.IORequest{Operation: "close", Path: "parent"}))
		}
		return identityErr
	}
	if ce != nil {
		return ioe(domain.IORequest{Operation: "close", Path: "parent"})
	}
	if !os.SameFile(t.parent, i) {
		return conflict(conflictRequest{Kind: "identity_changed"})
	}
	i, e = t.root.Lstat(t.name)
	if errors.Is(e, os.ErrNotExist) && t.target == nil {
		return nil
	}
	if e != nil {
		if errors.Is(e, os.ErrNotExist) {
			return conflict(conflictRequest{Kind: "identity_changed"})
		}
		return ioe(domain.IORequest{Operation: "identity", Path: "target"})
	}
	if t.target == nil || !os.SameFile(t.target, i) {
		return conflict(conflictRequest{Kind: "identity_changed"})
	}
	f, e := classify(classifyRequest{Root: t.root, Name: t.name, Kind: "target", Flags: os.O_RDONLY, Mode: 0})
	if e != nil {
		var de *domain.Error
		if errors.As(e, &de) {
			return e
		}
		return ioe(domain.IORequest{Operation: "identity", Path: "target"})
	}
	actual, e := f.Stat()
	ce = f.Close()
	if e != nil {
		return ioe(domain.IORequest{Operation: "identity", Path: "target"})
	}
	if ce != nil {
		return ioe(domain.IORequest{Operation: "close", Path: "target"})
	}
	if !os.SameFile(t.target, actual) {
		return conflict(conflictRequest{Kind: "identity_changed"})
	}
	return nil
}
func (t *Transaction) Commit(r CommitRequest) (err error) {
	if t.closed || !t.read {
		return ioe(domain.IORequest{Operation: "write", Path: "target"})
	}
	if int64(len(r.Bytes)) > domain.MaxTargetBytes {
		return validation(domain.ValidationRequest{Path: "target_size", Reason: "too_large"})
	}
	if err = t.identities(identitiesRequest{}); err != nil {
		return err
	}
	if len(r.Bytes) == 0 {
		if t.target == nil {
			return nil
		}
		if e := t.perform(storageAction{storageOperation: storageOperation{"unlink", "target"}, Action: func() error { return t.root.Remove(t.name) }}); e != nil {
			return ioe(domain.IORequest{Operation: "unlink", Path: "target"})
		}
	} else {
		var f *os.File
		var name string
		for n := 0; n < 16; n++ {
			var random [16]byte
			if _, e := rand.Read(random[:]); e != nil {
				return ioe(domain.IORequest{Operation: "csprng", Path: "temp"})
			}
			name = "." + t.name + "." + hex.EncodeToString(random[:]) + ".tmp"
			var e error
			f, e = t.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
			if e == nil {
				break
			}
			if !errors.Is(e, os.ErrExist) {
				return ioe(domain.IORequest{Operation: "open", Path: "temp"})
			}
		}
		if f == nil {
			return ioe(domain.IORequest{Operation: "randomness_exhausted", Path: "temp"})
		}
		defer func(request cleanupRequest) {
			if f != nil {
				if e := f.Close(); e != nil {
					err = ioe(domain.IORequest{Operation: "close", Path: "temp"})
				}
			}
			if name != "" {
				if e := t.root.Remove(name); e != nil {
					err = ioe(domain.IORequest{Operation: "cleanup", Path: "temp"})
				}
			}
		}(cleanupRequest{})
		i, e := f.Stat()
		var st syscall.Stat_t
		statErr := syscall.Fstat(int(f.Fd()), &st)
		if e != nil || statErr != nil || st.Nlink != 1 || !i.Mode().IsRegular() {
			return ioe(domain.IORequest{Operation: "identity", Path: "temp"})
		}
		if e = t.perform(storageAction{storageOperation: storageOperation{"write", "temp"}, Action: func() error { _, err := f.Write(r.Bytes); return err }}); e != nil {
			return ioe(domain.IORequest{Operation: "write", Path: "temp"})
		}
		if t.target != nil {
			if e = f.Chmod(t.target.Mode().Perm()); e != nil {
				return ioe(domain.IORequest{Operation: "write", Path: "temp"})
			}
		}
		if e = t.perform(storageAction{storageOperation: storageOperation{"sync", "temp"}, Action: f.Sync}); e != nil {
			return ioe(domain.IORequest{Operation: "write", Path: "temp"})
		}
		if e = t.closeFile(fileClose{storageOperation: storageOperation{"close", "temp"}, File: f}); e != nil {
			f = nil
			return ioe(domain.IORequest{Operation: "close", Path: "temp"})
		}
		f = nil
		if e = t.identities(identitiesRequest{}); e != nil {
			return e
		}
		current, e := t.root.Lstat(name)
		if e != nil || !os.SameFile(i, current) {
			return ioe(domain.IORequest{Operation: "identity", Path: "temp"})
		}
		if e = t.perform(storageAction{storageOperation: storageOperation{"rename", "target"}, Action: func() error { return t.root.Rename(name, t.name) }}); e != nil {
			return ioe(domain.IORequest{Operation: "rename", Path: "target"})
		}
		name = ""
	}
	dir, e := t.root.Open(".")
	if e != nil {
		return uncertain(uncertainRequest{})
	}
	e = t.perform(storageAction{storageOperation: storageOperation{"sync", "parent"}, Action: dir.Sync})
	ce := t.closeFile(fileClose{storageOperation: storageOperation{"close", "parent"}, File: dir})
	if e != nil {
		return uncertain(uncertainRequest{})
	}
	t.durable = true
	_ = ce
	return nil
}
func uncertain(r uncertainRequest) error {
	mayHaveSucceeded := true
	return domain.IO(domain.IORequest{Operation: "fsync_parent", Path: "parent", CommitMayHaveSucceeded: &mayHaveSucceeded})
}
func (t *Transaction) Close(r TransactionCloseRequest) (err error) {
	if t.closed {
		return nil
	}
	t.closed = true
	if t.lockFile != nil {
		if e := t.perform(storageAction{storageOperation: storageOperation{"unlock", "lock"}, Action: func() error { return syscall.Flock(int(t.lockFile.Fd()), syscall.LOCK_UN) }}); e != nil {
			err = ioe(domain.IORequest{Operation: "unlock", Path: "lock"})
		}
		if e := t.closeFile(fileClose{storageOperation: storageOperation{"close", "lock"}, File: t.lockFile}); e != nil && err == nil {
			err = ioe(domain.IORequest{Operation: "close", Path: "lock"})
		}
	}
	if t.root != nil {
		if e := t.root.Close(); e != nil && err == nil {
			err = ioe(domain.IORequest{Operation: "close", Path: "parent"})
		}
	}
	if t.durable {
		return nil
	}
	return err
}
