package operations

import (
	"encoding/json"
	"fmt"
	"github.com/trogers1/sidecar-comments/internal/domain"
)

type OperationDTO struct {
	Op               string          `json:"op"`
	ThreadID         string          `json:"thread_id"`
	MessageID        string          `json:"message_id,omitempty"`
	Location         domain.Location `json:"location,omitempty"`
	Author           domain.Author   `json:"author,omitempty"`
	Body             string          `json:"body,omitempty"`
	GitLab           *domain.GitLab  `json:"gitlab,omitempty"`
	IfLastAuthorKind string          `json:"if_last_author_kind,omitempty"`
}
type Result struct {
	Thread           *domain.Thread  `json:"thread,omitempty"`
	Message          *domain.Message `json:"message,omitempty"`
	DeletedThreadID  string          `json:"deleted_thread_id,omitempty"`
	DeletedMessageID string          `json:"deleted_message_id,omitempty"`
}
type Request struct {
	Document  domain.DocumentValue
	Operation Value
	Index     *int
}
type Response struct {
	Document domain.DocumentValue
	Result   Result
	Changed  bool
}
type failRequest struct{ Code, Reason, ID string }
type notFoundDetails struct {
	Entity string `json:"entity"`
	ID     string `json:"id"`
	Index  *int   `json:"index,omitempty"`
}
type PreconditionDetails struct {
	Operation string `json:"operation"`
	Reason    string `json:"reason"`
	Index     *int   `json:"index,omitempty"`
}

func Execute(r Request) (Response, error) {
	if !r.Operation.valid {
		return Response{}, fmt.Errorf("uninitialized operation value")
	}
	threads := r.Document.Entries(domain.DocumentEntriesRequest{})
	o := r.Operation.dto
	fail := func(f failRequest) (Response, error) {
		var details []byte
		var err error
		message, exit := "precondition:"+o.Op+":"+f.Reason, domain.ExitPrecondition
		if f.Code == domain.CodeNotFound {
			details, err = json.Marshal(notFoundDetails{Entity: f.Reason, ID: f.ID, Index: r.Index})
			message = "not_found:" + f.Reason
			exit = domain.ExitNotFound
		} else {
			details, err = json.Marshal(PreconditionDetails{Operation: o.Op, Reason: f.Reason, Index: r.Index})
		}
		if err != nil {
			return Response{}, fmt.Errorf("encode operation error: %w", err)
		}
		return Response{}, &domain.Error{Code: f.Code, Message: message, Details: details, Exit: exit}
	}
	ti, mi := -1, -1
	messageExists := false
	for i, t := range threads {
		if t.ID(domain.ValueRequest{}) == r.Operation.threadID {
			ti = i
		}
		for j, m := range t.Messages(domain.ValueRequest{}) {
			if m.ID(domain.ValueRequest{}) == r.Operation.messageID {
				messageExists = true
				if t.ID(domain.ValueRequest{}) == r.Operation.threadID {
					mi = j
				}
			}
		}
	}
	response := Response{Document: r.Document}
	var resultThread *domain.ThreadValue
	var resultMessage *domain.MessageValue
	switch o.Op {
	case domain.OpThreadCreate:
		message, err := domain.NewMessageValue(domain.NewMessageValueRequest{ID: r.Operation.messageID, Author: o.Author, Body: o.Body, GitLab: o.GitLab})
		if err != nil {
			return Response{}, err
		}
		thread, err := domain.NewThreadValue(domain.NewThreadValueRequest{ID: r.Operation.threadID, Location: o.Location, Messages: []domain.MessageValue{message}})
		if err != nil {
			return Response{}, err
		}
		if ti >= 0 {
			if !equalThreads(equalThreadsRequest{Left: threads[ti], Right: thread}) {
				return fail(failRequest{Code: domain.CodePrecondition, Reason: "retry_mismatch"})
			}
			dto := threads[ti].DTO(domain.ValueRequest{})
			response.Result.Thread = &dto
			return response, nil
		}
		if messageExists {
			return fail(failRequest{Code: domain.CodePrecondition, Reason: "id_collision"})
		}
		threads = append(threads, thread)
		resultThread = &thread
		response.Changed = true
	case domain.OpThreadMove:
		if ti < 0 {
			return fail(failRequest{Code: domain.CodeNotFound, Reason: "thread", ID: o.ThreadID})
		}
		previous := threads[ti]
		response.Changed = !equal(equalRequest[domain.Location]{Left: previous.Location(domain.ValueRequest{}), Right: o.Location})
		thread, err := domain.NewThreadValue(domain.NewThreadValueRequest{ID: previous.ID(domain.ValueRequest{}), Location: o.Location, Messages: previous.Messages(domain.ValueRequest{})})
		if err != nil {
			return Response{}, err
		}
		threads[ti] = thread
		resultThread = &thread
	case domain.OpThreadDelete:
		if ti < 0 {
			return fail(failRequest{Code: domain.CodeNotFound, Reason: "thread", ID: o.ThreadID})
		}
		threads = append(threads[:ti], threads[ti+1:]...)
		response.Result.DeletedThreadID = o.ThreadID
		response.Changed = true
	case domain.OpMessageAdd:
		if ti < 0 {
			return fail(failRequest{Code: domain.CodeNotFound, Reason: "thread", ID: o.ThreadID})
		}
		previous := threads[ti]
		messages := previous.Messages(domain.ValueRequest{})
		if o.IfLastAuthorKind != "" && messages[len(messages)-1].Author(domain.ValueRequest{}).Kind != o.IfLastAuthorKind {
			return fail(failRequest{Code: domain.CodePrecondition, Reason: "if_last_author_kind"})
		}
		if messageExists {
			return fail(failRequest{Code: domain.CodePrecondition, Reason: "id_collision"})
		}
		if len(messages) >= domain.MaxMessagesThread {
			return fail(failRequest{Code: domain.CodePrecondition, Reason: "limit_messages_thread"})
		}
		message, err := domain.NewMessageValue(domain.NewMessageValueRequest{ID: r.Operation.messageID, Author: o.Author, Body: o.Body, GitLab: o.GitLab})
		if err != nil {
			return Response{}, err
		}
		messages = append(messages, message)
		thread, err := domain.NewThreadValue(domain.NewThreadValueRequest{ID: previous.ID(domain.ValueRequest{}), Location: previous.Location(domain.ValueRequest{}), Messages: messages})
		if err != nil {
			return Response{}, err
		}
		threads[ti] = thread
		resultThread = &thread
		resultMessage = &message
		response.Changed = true
	case domain.OpMessageUpdate:
		if ti < 0 {
			return fail(failRequest{Code: domain.CodeNotFound, Reason: "thread", ID: o.ThreadID})
		}
		if mi < 0 {
			return fail(failRequest{Code: domain.CodeNotFound, Reason: "message", ID: o.MessageID})
		}
		previous := threads[ti]
		messages := previous.Messages(domain.ValueRequest{})
		old := messages[mi]
		response.Changed = old.Body(domain.ValueRequest{}) != o.Body
		message, err := domain.NewMessageValue(domain.NewMessageValueRequest{ID: old.ID(domain.ValueRequest{}), Author: old.Author(domain.ValueRequest{}), Body: o.Body, GitLab: old.GitLab(domain.ValueRequest{})})
		if err != nil {
			return Response{}, err
		}
		messages[mi] = message
		thread, err := domain.NewThreadValue(domain.NewThreadValueRequest{ID: previous.ID(domain.ValueRequest{}), Location: previous.Location(domain.ValueRequest{}), Messages: messages})
		if err != nil {
			return Response{}, err
		}
		threads[ti] = thread
		resultThread = &thread
		resultMessage = &message
	case domain.OpMessageDelete:
		if ti < 0 {
			return fail(failRequest{Code: domain.CodeNotFound, Reason: "thread", ID: o.ThreadID})
		}
		if mi < 0 {
			return fail(failRequest{Code: domain.CodeNotFound, Reason: "message", ID: o.MessageID})
		}
		previous := threads[ti]
		messages := previous.Messages(domain.ValueRequest{})
		messages = append(messages[:mi], messages[mi+1:]...)
		response.Result.DeletedMessageID = o.MessageID
		if len(messages) == 0 {
			threads = append(threads[:ti], threads[ti+1:]...)
			response.Result.DeletedThreadID = o.ThreadID
		} else {
			thread, err := domain.NewThreadValue(domain.NewThreadValueRequest{ID: previous.ID(domain.ValueRequest{}), Location: previous.Location(domain.ValueRequest{}), Messages: messages})
			if err != nil {
				return Response{}, err
			}
			threads[ti] = thread
		}
		response.Changed = true
	default:
		return Response{}, fmt.Errorf("invalid validated operation tag %q", o.Op)
	}
	if len(threads) > domain.MaxThreads {
		return fail(failRequest{Code: domain.CodePrecondition, Reason: "limit_threads"})
	}
	total := 0
	for _, t := range threads {
		total += len(t.Messages(domain.ValueRequest{}))
	}
	if total > domain.MaxMessagesDocument {
		return fail(failRequest{Code: domain.CodePrecondition, Reason: "limit_messages_document"})
	}
	value, err := domain.NewDocumentFromValues(domain.NewDocumentFromValuesRequest{Threads: threads})
	if err != nil {
		return Response{}, err
	}
	response.Document = value
	// DTO conversion is confined to the public result serialization boundary.
	if resultThread != nil {
		dto := resultThread.DTO(domain.ValueRequest{})
		response.Result.Thread = &dto
	}
	if resultMessage != nil {
		dto := resultMessage.DTO(domain.ValueRequest{})
		response.Result.Message = &dto
	}
	return response, nil
}
