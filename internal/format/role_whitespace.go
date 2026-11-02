package format

import (
	"github.com/trogers1/sidecar-comments/internal/domain"
	"strings"
)

type roleWhitespaceRequest struct{ Text string }

// Only horizontal ASCII whitespace at the end of a recognized role header is
// insignificant. Body bytes, other delimiters, and all other whitespace remain strict.
func trimRoleWhitespace(r roleWhitespaceRequest) string {
	trimmed := strings.TrimRight(r.Text, " \t")
	for _, name := range []string{domain.RoleUser, domain.RoleBot, domain.RoleExternal} {
		prefix := "---" + name
		if trimmed == prefix || strings.HasPrefix(trimmed, prefix+" <!-- sidecar-meta: ") {
			return trimmed
		}
	}
	return r.Text
}
