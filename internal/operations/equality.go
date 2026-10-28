package operations

import (
	"bytes"
	"encoding/json"
	"github.com/trogers1/sidecar-comments/internal/domain"
)

type comparableDTO interface {
	domain.Location | domain.Author | *domain.GitLab
}
type equalRequest[T comparableDTO] struct{ Left, Right T }

// These closed field shapes contain only serializable fields. Serialization
// failure is an internal invariant violation handled by the exit-7 boundary.
func equal[T comparableDTO](r equalRequest[T]) bool {
	left, err := json.Marshal(r.Left)
	if err != nil {
		panic(err)
	}
	right, err := json.Marshal(r.Right)
	if err != nil {
		panic(err)
	}
	return bytes.Equal(left, right)
}

type equalThreadsRequest struct{ Left, Right domain.ThreadValue }

func equalThreads(r equalThreadsRequest) bool {
	if r.Left.ID(domain.ValueRequest{}) != r.Right.ID(domain.ValueRequest{}) || !equal(equalRequest[domain.Location]{Left: r.Left.Location(domain.ValueRequest{}), Right: r.Right.Location(domain.ValueRequest{})}) {
		return false
	}
	left, right := r.Left.Messages(domain.ValueRequest{}), r.Right.Messages(domain.ValueRequest{})
	if len(left) != len(right) {
		return false
	}
	for i, l := range left {
		other := right[i]
		if l.ID(domain.ValueRequest{}) != other.ID(domain.ValueRequest{}) || l.Body(domain.ValueRequest{}) != other.Body(domain.ValueRequest{}) || !equal(equalRequest[domain.Author]{Left: l.Author(domain.ValueRequest{}), Right: other.Author(domain.ValueRequest{})}) || !equal(equalRequest[*domain.GitLab]{Left: l.GitLab(domain.ValueRequest{}), Right: other.GitLab(domain.ValueRequest{})}) {
			return false
		}
	}
	return true
}
