package format

import (
	"strings"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

type trailingPaddingRequest struct{ Text string }

// Ignore only ASCII horizontal whitespace and blank lines after a final closing
// delimiter. At least one LF must follow its bracket. The parser still checks
// the closing ID, and no body bytes or internal delimiters are normalized.
func trimTrailingPadding(r trailingPaddingRequest) string {
	trimmed := strings.TrimRight(r.Text, " \t\n")
	suffix := r.Text[len(trimmed):]
	if !strings.Contains(suffix, "\n") {
		return r.Text
	}
	lastLine := trimmed[strings.LastIndex(trimmed, "\n")+1:]
	if !strings.HasPrefix(lastLine, domain.ThreadClosePrefix) || !strings.HasSuffix(lastLine, "]") {
		return r.Text
	}
	return trimmed + "\n"
}
