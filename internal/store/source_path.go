package store

import "strings"

type DerivedSourcePathRequest struct{ TargetPath string }

// DerivedSourcePath validates naming without opening or parsing the sidecar.
func DerivedSourcePath(r DerivedSourcePathRequest) (string, error) {
	if err := validateTargetPath(pathRequest{Path: r.TargetPath}); err != nil {
		return "", err
	}
	return strings.TrimSuffix(r.TargetPath, ".comments"), nil
}
