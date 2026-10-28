package domain

// Clone helpers detach untrusted wire DTOs without validating them.
type CloneLocationRequest struct{ Location Location }
type CloneAuthorRequest struct{ Author Author }
type CloneGitLabRequest struct{ GitLab *GitLab }

func CloneLocation(r CloneLocationRequest) Location {
	return cloneLocation(cloneLocationRequest{Location: r.Location})
}
func CloneAuthor(r CloneAuthorRequest) Author {
	return cloneAuthor(cloneAuthorRequest{Author: r.Author})
}
func CloneGitLab(r CloneGitLabRequest) *GitLab {
	return cloneGitLab(cloneGitLabRequest{GitLab: r.GitLab})
}
