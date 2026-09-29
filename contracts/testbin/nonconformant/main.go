// nonconformant is intentionally wrong. It is used only to prove that the
// black-box corpus rejects a compiled executable, not as a Sidecar substitute.
package main

import "fmt"

func main() { fmt.Println("this executable intentionally does not implement Sidecar v1") }
