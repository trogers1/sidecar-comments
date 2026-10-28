package main

import (
	"github.com/trogers1/sidecar-comments/internal/cli"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	// Let checked delivery report EPIPE as exit 7, not signal termination.
	signal.Ignore(syscall.SIGPIPE)
	os.Exit(cli.Run(cli.Request{Args: os.Args[1:], Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr}))
}
