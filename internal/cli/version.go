package cli

import "regexp"

// buildVersion is set only by the release build's authoritative Git identity.
var buildVersion = "dev"

var versionPattern = regexp.MustCompile(`^(dev|0\.0\.0-main\.[0-9a-f]{40}|(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*))$`)

// Version returns the build identity, rejecting malformed linker input.
func Version() string {
	if !versionPattern.MatchString(buildVersion) {
		panic("invalid CLI build version: " + buildVersion)
	}
	return buildVersion
}
