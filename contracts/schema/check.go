// Command check validates the checked-in contract corpus before an executable is run.
package main

import (
	"fmt"
	"os"

	"github.com/trogers1/sidecar-comments/contracts/runner"
)

func main() {
	if err := runner.ValidateArtifacts("contracts/corpus/v1.json"); err != nil {
		fmt.Fprintf(os.Stderr, "contract artifacts: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("contract artifacts: corpus, schemas, fixture references, coverage, and goldens are coherent")
}
