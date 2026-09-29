// conforming is a deliberately tiny test-only subject. It proves that the
// harness can accept a correctly behaving child process for one controlled
// fixture; it is not a Sidecar implementation.
package main

import (
	"fmt"
	"os"
)

const (
	jsonVersion = "{\"protocol\":1,\"command\":\"version\",\"changed\":false,\"result\":{\"cli_version\":\"test\",\"format_version\":1,\"protocol\":1}}\n"
	textVersion = "version changed=false result={\"cli_version\":\"test\",\"format_version\":1,\"protocol\":1}\n"
)

func main() {
	if len(os.Args) == 4 && os.Args[1] == "--output" && os.Args[2] == "json" && os.Args[3] == "version" {
		fmt.Print(jsonVersion)
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "version" {
		fmt.Print(textVersion)
		return
	}
	fmt.Fprintln(os.Stderr, "test-only conforming subject received an unsupported fixture")
	os.Exit(2)
}
