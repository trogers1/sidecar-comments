package domain

// IDs have distinct, constructor-controlled namespaces. Their zero values are invalid.
type ThreadID struct{ text string }
type MessageID struct{ text string }
type NewThreadIDRequest struct{ Text string }
type NewMessageIDRequest struct{ Text string }
type IDTextRequest struct{}

func NewThreadID(r NewThreadIDRequest) (ThreadID, error) {
	if err := ValidateID(ValidateIDRequest{ID: r.Text}); err != nil {
		return ThreadID{}, err
	}
	return ThreadID{text: r.Text}, nil
}
func NewMessageID(r NewMessageIDRequest) (MessageID, error) {
	if err := ValidateID(ValidateIDRequest{ID: r.Text}); err != nil {
		return MessageID{}, err
	}
	return MessageID{text: r.Text}, nil
}
func (id ThreadID) Text(r IDTextRequest) string  { return id.text }
func (id MessageID) Text(r IDTextRequest) string { return id.text }

type ValueRequest struct{}
type MessageValue struct {
	id     MessageID
	author Author
	body   string
	gitlab *GitLab
	valid  bool
}
type ThreadValue struct {
	id       ThreadID
	location Location
	messages []MessageValue
	valid    bool
}

// The zero document is a valid empty document.
type DocumentValue struct{ threads []ThreadValue }
type NewMessageValueRequest struct {
	ID     MessageID
	Author Author
	Body   string
	GitLab *GitLab
}
type NewThreadValueRequest struct {
	ID       ThreadID
	Location Location
	Messages []MessageValue
}
type NewDocumentFromValuesRequest struct{ Threads []ThreadValue }
type NewDocumentValueRequest struct{ Threads []Thread }
type DocumentEntriesRequest struct{}
type DocumentThreadsRequest struct{}

type cloneLocationRequest struct{ Location Location }

func cloneLocation(r cloneLocationRequest) Location {
	l := r.Location
	if l.Anchor != nil {
		a := *l.Anchor
		l.Anchor = &a
	}
	return l
}

type cloneAuthorRequest struct{ Author Author }

func cloneAuthor(r cloneAuthorRequest) Author {
	a := r.Author
	if a.Name != nil {
		n := *a.Name
		a.Name = &n
	}
	if a.ExternalAuthor != nil {
		e := *a.ExternalAuthor
		a.ExternalAuthor = &e
	}
	return a
}

type cloneGitLabRequest struct{ GitLab *GitLab }

func cloneGitLab(r cloneGitLabRequest) *GitLab {
	if r.GitLab == nil {
		return nil
	}
	g := *r.GitLab
	if g.DiscussionID != nil {
		d := *g.DiscussionID
		g.DiscussionID = &d
	}
	return &g
}

func NewMessageValue(r NewMessageValueRequest) (MessageValue, error) {
	if err := ValidateID(ValidateIDRequest{ID: r.ID.text}); err != nil {
		return MessageValue{}, err
	}
	if err := ValidateAuthor(ValidateAuthorRequest{Author: r.Author}); err != nil {
		return MessageValue{}, err
	}
	if err := ValidateBody(ValidateBodyRequest{Body: r.Body}); err != nil {
		return MessageValue{}, err
	}
	if err := ValidateGitLab(ValidateGitLabRequest{GitLab: r.GitLab}); err != nil {
		return MessageValue{}, err
	}
	return MessageValue{id: r.ID, author: cloneAuthor(cloneAuthorRequest{Author: r.Author}), body: r.Body, gitlab: cloneGitLab(cloneGitLabRequest{GitLab: r.GitLab}), valid: true}, nil
}
func NewThreadValue(r NewThreadValueRequest) (ThreadValue, error) {
	if err := ValidateID(ValidateIDRequest{ID: r.ID.text}); err != nil {
		return ThreadValue{}, err
	}
	if err := ValidateLocation(ValidateLocationRequest{Location: r.Location}); err != nil {
		return ThreadValue{}, err
	}
	if len(r.Messages) == 0 || len(r.Messages) > MaxMessagesThread {
		return ThreadValue{}, invalid(invalidRequest{})
	}
	seen := map[MessageID]bool{}
	for _, m := range r.Messages {
		if !m.valid || seen[m.id] {
			return ThreadValue{}, invalid(invalidRequest{})
		}
		seen[m.id] = true
	}
	return ThreadValue{id: r.ID, location: cloneLocation(cloneLocationRequest{Location: r.Location}), messages: append([]MessageValue(nil), r.Messages...), valid: true}, nil
}
func NewDocumentFromValues(r NewDocumentFromValuesRequest) (DocumentValue, error) {
	if len(r.Threads) > MaxThreads {
		return DocumentValue{}, invalid(invalidRequest{})
	}
	threads := map[ThreadID]bool{}
	messages := map[MessageID]bool{}
	total := 0
	for _, t := range r.Threads {
		if !t.valid || threads[t.id] {
			return DocumentValue{}, invalid(invalidRequest{})
		}
		threads[t.id] = true
		total += len(t.messages)
		if total > MaxMessagesDocument {
			return DocumentValue{}, invalid(invalidRequest{})
		}
		for _, m := range t.messages {
			if !m.valid || messages[m.id] {
				return DocumentValue{}, invalid(invalidRequest{})
			}
			messages[m.id] = true
		}
	}
	return DocumentValue{threads: append([]ThreadValue(nil), r.Threads...)}, nil
}
func NewDocumentValue(r NewDocumentValueRequest) (DocumentValue, error) {
	if len(r.Threads) > MaxThreads {
		return DocumentValue{}, invalid(invalidRequest{})
	}
	threads := make([]ThreadValue, len(r.Threads))
	for i, t := range r.Threads {
		tid, err := NewThreadID(NewThreadIDRequest{Text: t.ID})
		if err != nil {
			return DocumentValue{}, err
		}
		if len(t.Messages) == 0 || len(t.Messages) > MaxMessagesThread {
			return DocumentValue{}, invalid(invalidRequest{})
		}
		messages := make([]MessageValue, len(t.Messages))
		for j, m := range t.Messages {
			mid, err := NewMessageID(NewMessageIDRequest{Text: m.ID})
			if err != nil {
				return DocumentValue{}, err
			}
			messages[j], err = NewMessageValue(NewMessageValueRequest{ID: mid, Author: m.Author, Body: m.Body, GitLab: m.GitLab})
			if err != nil {
				return DocumentValue{}, err
			}
		}
		threads[i], err = NewThreadValue(NewThreadValueRequest{ID: tid, Location: t.Location, Messages: messages})
		if err != nil {
			return DocumentValue{}, err
		}
	}
	return NewDocumentFromValues(NewDocumentFromValuesRequest{Threads: threads})
}
func (m MessageValue) ID(r ValueRequest) MessageID { return m.id }
func (m MessageValue) Author(r ValueRequest) Author {
	return cloneAuthor(cloneAuthorRequest{Author: m.author})
}
func (m MessageValue) Body(r ValueRequest) string { return m.body }
func (m MessageValue) GitLab(r ValueRequest) *GitLab {
	return cloneGitLab(cloneGitLabRequest{GitLab: m.gitlab})
}
func (m MessageValue) DTO(r ValueRequest) Message {
	return Message{ID: m.id.Text(IDTextRequest{}), Author: m.Author(ValueRequest{}), Body: m.body, GitLab: m.GitLab(ValueRequest{})}
}
func (t ThreadValue) ID(r ValueRequest) ThreadID { return t.id }
func (t ThreadValue) Location(r ValueRequest) Location {
	return cloneLocation(cloneLocationRequest{Location: t.location})
}
func (t ThreadValue) Messages(r ValueRequest) []MessageValue {
	return append([]MessageValue(nil), t.messages...)
}
func (t ThreadValue) DTO(r ValueRequest) Thread {
	messages := make([]Message, len(t.messages))
	for i, m := range t.messages {
		messages[i] = m.DTO(ValueRequest{})
	}
	return Thread{ID: t.id.Text(IDTextRequest{}), Location: t.Location(ValueRequest{}), Messages: messages}
}
func (d DocumentValue) Entries(r DocumentEntriesRequest) []ThreadValue {
	return append([]ThreadValue(nil), d.threads...)
}

// Threads exports defensive wire DTOs only for serialization boundaries.
func (d DocumentValue) Threads(r DocumentThreadsRequest) []Thread {
	threads := make([]Thread, len(d.threads))
	for i, t := range d.threads {
		threads[i] = t.DTO(ValueRequest{})
	}
	return threads
}
