package format

import (
	"strconv"
	"strings"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type unsupportedHeaderRequest struct{ Text string }

// Version recognition is lexical so even an unrepresentably large positive
// version fails closed as unsupported rather than overflowing a machine int.
func unsupportedHeader(r unsupportedHeaderRequest) bool {
	const prefix = "<!-- sidecar-format: "
	if !strings.HasPrefix(r.Text, prefix) || !strings.HasSuffix(r.Text, commentSuffix) {
		return false
	}
	version := strings.TrimSuffix(strings.TrimPrefix(r.Text, prefix), commentSuffix)
	if len(version) == 0 || version[0] < '1' || version[0] > '9' {
		return false
	}
	for _, digit := range version {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return version != strconv.Itoa(domain.FormatVersion)
}
