package cli

import "github.com/trogers1/sidecar-comments/internal/domain"

type uniqueIDRequest struct {
	Document      domain.DocumentValue
	RepairThreads []domain.Thread
	Namespace     string
}

func uniqueID(r uniqueIDRequest) (string, error) {
	threads := map[domain.ThreadID]bool{}
	messages := map[domain.MessageID]bool{}
	for _, t := range r.Document.Entries(domain.DocumentEntriesRequest{}) {
		threads[t.ID(domain.ValueRequest{})] = true
		for _, m := range t.Messages(domain.ValueRequest{}) {
			messages[m.ID(domain.ValueRequest{})] = true
		}
	}
	for _, t := range r.RepairThreads {
		tid, e := domain.NewThreadID(domain.NewThreadIDRequest{Text: t.ID})
		if e != nil {
			return "", e
		}
		threads[tid] = true
		for _, m := range t.Messages {
			if m.ID == "" {
				continue
			}
			mid, e := domain.NewMessageID(domain.NewMessageIDRequest{Text: m.ID})
			if e != nil {
				return "", e
			}
			messages[mid] = true
		}
	}
	for attempt := 0; attempt < 16; attempt++ {
		text, e := domain.NewID(domain.NewIDRequest{})
		if e != nil {
			return "", e
		}
		switch r.Namespace {
		case "thread":
			id, e := domain.NewThreadID(domain.NewThreadIDRequest{Text: text})
			if e != nil {
				return "", e
			}
			if !threads[id] {
				return text, nil
			}
		case "message":
			id, e := domain.NewMessageID(domain.NewMessageIDRequest{Text: text})
			if e != nil {
				return "", e
			}
			if !messages[id] {
				return text, nil
			}
		default:
			return "", domain.Validation(domain.ValidationRequest{Path: "target_content", Reason: "invalid"})
		}
	}
	return "", domain.IO(domain.IORequest{Operation: "randomness_exhausted", Path: "generator"})
}
