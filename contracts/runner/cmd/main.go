// sidecar-contract runs the checked-in black-box executable contract corpus.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/trogers1/sidecar-comments/contracts/runner"
)

func main() {
	var corpus, bin, version string
	var timeout time.Duration
	var requiredRed bool
	flag.StringVar(&corpus, "corpus", "contracts/corpus/v1.json", "corpus manifest")
	flag.StringVar(&bin, "bin", "", "executable (defaults to SIDECAR_BIN then manifest)")
	flag.StringVar(&version, "cli-version", "", "value for {{CLI_VERSION}} templates")
	flag.DurationVar(&timeout, "timeout", 10*time.Second, "per-fixture timeout")
	flag.BoolVar(&requiredRed, "required-red", false, "report unavailable mandatory fixture packs and exit nonzero")
	flag.Parse()
	if requiredRed {
		obligations, err := runner.RequiredRedObligations(corpus)
		if err != nil {
			fmt.Fprintf(os.Stderr, "required-red inventory: %v\n", err)
			os.Exit(1)
		}
		if len(obligations) != 0 {
			fmt.Fprintln(os.Stderr, "full-red unavailable; mandatory fixture packs are missing:")
			for _, obligation := range obligations {
				fmt.Fprintf(os.Stderr, "  %s\n", obligation)
			}
			os.Exit(2)
		}
		return
	}
	r, e := runner.Run(runner.Config{CorpusPath: corpus, Binary: bin, CLIVersion: version, Timeout: timeout})
	if e != nil {
		fmt.Fprintf(os.Stderr, "contract runner: %v\n", e)
		for _, f := range r.Failures {
			fmt.Fprintf(os.Stderr, "  %s: %s\n", f.ID, f.Reason)
		}
		os.Exit(1)
	}
}
