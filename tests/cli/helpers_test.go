package cli_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"time"
)

type failureStream uint8

const (
	noStreamFailure failureStream = iota
	stdoutFailure
	stderrFailure
)

type runRequest struct {
	Binary      string
	Args        []string
	Dir         string
	Stdin       []byte
	Timeout     time.Duration
	OutputLimit int
	FailOutput  failureStream
}

type processResult struct {
	Status         int
	Stdout, Stderr []byte
}

// Each stream is drained by its own exec copy goroutine. Results are read only
// after Wait has joined both; overflow notification never blocks a writer.
type boundedOutput struct {
	data     []byte
	limit    int
	overflow chan struct{}
}

func (w *boundedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := w.limit - len(w.data)
	if n > remaining {
		w.data = append(w.data, p[:remaining]...)
		select {
		case w.overflow <- struct{}{}:
		default:
		}
		return remaining, errors.New("process output exceeds limit")
	}
	w.data = append(w.data, p...)
	return n, nil
}

// This boundary is intentionally macOS-only for Stage A. Process groups cover
// ordinary descendants, not hostile children deliberately escaping the group.
func runProcess(r runRequest) (processResult, error) {
	if runtime.GOOS != "darwin" {
		return processResult{}, errors.New("CLI helper currently requires macOS; Linux is deferred")
	}
	if r.Binary == "" || r.Dir == "" || r.Timeout <= 0 || r.Timeout > time.Minute || r.OutputLimit <= 0 || r.OutputLimit > 1<<20 {
		return processResult{}, errors.New("binary and directory required; timeout must be within (0, 1m] and output limit within [1, 1048576]")
	}
	cmd := exec.Command(r.Binary, r.Args...)
	cmd.Dir = r.Dir
	cmd.Stdin = bytes.NewReader(r.Stdin)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = time.Second
	overflow := make(chan struct{}, 1)
	stdout := &boundedOutput{limit: r.OutputLimit, overflow: overflow}
	stderr := &boundedOutput{limit: r.OutputLimit, overflow: overflow}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	// A valid pipe with no reader deterministically fails writes with EPIPE.
	// Closing descriptor 1/2 instead is unreliable: runtimes may repair them.
	var failedOutput *os.File
	switch r.FailOutput {
	case noStreamFailure:
	case stdoutFailure, stderrFailure:
		reader, writer, err := os.Pipe()
		if err != nil {
			return processResult{}, fmt.Errorf("create failed output pipe: %w", err)
		}
		if err := reader.Close(); err != nil {
			return processResult{}, errors.Join(fmt.Errorf("close failed output reader: %w", err), writer.Close())
		}
		failedOutput = writer
		if r.FailOutput == stdoutFailure {
			cmd.Stdout = writer
		} else {
			cmd.Stderr = writer
		}
	default:
		return processResult{}, errors.New("unknown output failure stream")
	}
	if err := cmd.Start(); err != nil {
		if failedOutput != nil {
			err = errors.Join(err, failedOutput.Close())
		}
		return processResult{}, fmt.Errorf("start process: %w", err)
	}
	var pipeCloseErr error
	if failedOutput != nil {
		pipeCloseErr = failedOutput.Close()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	timer := time.NewTimer(r.Timeout)
	defer timer.Stop()
	var waitErr error
	boundaryErr := pipeCloseErr
	waited := false
	select {
	case waitErr = <-done:
		waited = true
	case <-timer.C:
		boundaryErr = errors.Join(boundaryErr, errors.New("process timeout"))
	case <-overflow:
		boundaryErr = errors.Join(boundaryErr, errors.New("process output exceeds limit"))
	}
	// Also kill lingering descendants after an ordinary parent exit. Always
	// reap the direct child and join stream copies before observing output.
	killErr := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if killErr != nil && !errors.Is(killErr, syscall.ESRCH) {
		boundaryErr = errors.Join(boundaryErr, fmt.Errorf("terminate process group: %w", killErr))
	}
	if !waited {
		waitErr = <-done
	}
	result := processResult{Status: cmd.ProcessState.ExitCode(), Stdout: stdout.data, Stderr: stderr.data}
	select {
	case <-overflow:
		boundaryErr = errors.Join(boundaryErr, errors.New("process output exceeds limit"))
	default:
	}
	if boundaryErr != nil {
		return result, boundaryErr
	}
	// A normal nonzero status is an observation, not a helper failure.
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		return result, fmt.Errorf("wait process: %w", waitErr)
	}
	return result, nil
}

type processExpectation struct{ Got, Want processResult }

func compareProcess(r processExpectation) error {
	if r.Got.Status != r.Want.Status || !bytes.Equal(r.Got.Stdout, r.Want.Stdout) || !bytes.Equal(r.Got.Stderr, r.Want.Stderr) {
		return fmt.Errorf("process mismatch: got status=%d stdout=%q stderr=%q; want status=%d stdout=%q stderr=%q", r.Got.Status, r.Got.Stdout, r.Got.Stderr, r.Want.Status, r.Want.Stdout, r.Want.Stderr)
	}
	return nil
}

type fileRequest struct {
	Path  string
	Mode  os.FileMode
	Bytes []byte
}

func writeFile(r fileRequest) error {
	if len(r.Bytes) > 1<<20 || r.Mode & ^os.FileMode(0777) != 0 {
		return errors.New("fixture requires at most 1 MiB and permission bits only")
	}
	f, err := os.OpenFile(r.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, r.Mode)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(r.Bytes)
	// Set exact fixture mode independently of the caller's umask.
	modeErr := f.Chmod(r.Mode)
	return errors.Join(writeErr, modeErr, f.Close())
}

type filesRequest struct {
	Dir       string
	ByteLimit int64
}

// Only immediate regular files are needed for the first lifecycle. Unknown
// objects fail explicitly rather than disappearing from a filesystem oracle.
func observeFiles(r filesRequest) ([]fileRequest, error) {
	if r.ByteLimit <= 0 || r.ByteLimit > 1<<20 {
		return nil, errors.New("file byte limit must be within [1, 1048576]")
	}
	directory, err := os.Open(r.Dir)
	if err != nil {
		return nil, err
	}
	entries, readErr := directory.ReadDir(33)
	if errors.Is(readErr, io.EOF) {
		readErr = nil
	}
	if err := errors.Join(readErr, directory.Close()); err != nil {
		return nil, err
	}
	if len(entries) > 32 {
		return nil, errors.New("observation exceeds 32 files")
	}
	// ReadDir on an open directory returns filesystem order, not name order.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var files []fileRequest
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("nonregular fixture path: %s", entry.Name())
		}
		f, err := os.Open(filepath.Join(r.Dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(f, r.ByteLimit+1))
		if err := errors.Join(readErr, f.Close()); err != nil {
			return nil, err
		}
		if int64(len(data)) > r.ByteLimit {
			return nil, fmt.Errorf("file exceeds observation limit: %s", entry.Name())
		}
		files = append(files, fileRequest{Path: entry.Name(), Mode: info.Mode(), Bytes: data})
	}
	return files, nil
}

type filesExpectation struct{ Got, Want []fileRequest }

func compareFiles(r filesExpectation) error {
	if len(r.Got) != len(r.Want) {
		return fmt.Errorf("file count: got %d want %d", len(r.Got), len(r.Want))
	}
	for i, got := range r.Got {
		want := r.Want[i]
		if got.Path != want.Path || got.Mode != want.Mode || !bytes.Equal(got.Bytes, want.Bytes) {
			return fmt.Errorf("file mismatch: got path=%q mode=%v bytes=%q; want path=%q mode=%v bytes=%q", got.Path, got.Mode, got.Bytes, want.Path, want.Mode, want.Bytes)
		}
	}
	return nil
}
