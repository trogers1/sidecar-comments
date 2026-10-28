package domain_test

import (
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

func TestLocationConstructorRejectsInvalidClosedUnion(t *testing.T) {
	anchor := "YQ=="
	text, err := domain.NewID(domain.NewIDRequest{})
	if err != nil {
		t.Fatal(err)
	}
	tid, err := domain.NewThreadID(domain.NewThreadIDRequest{Text: text})
	if err != nil {
		t.Fatal(err)
	}
	mid, err := domain.NewMessageID(domain.NewMessageIDRequest{Text: text})
	if err != nil {
		t.Fatal(err)
	}
	message, err := domain.NewMessageValue(domain.NewMessageValueRequest{ID: mid, Author: domain.Author{Kind: domain.AuthorHuman}, Body: "body"})
	if err != nil {
		t.Fatal(err)
	}
	for _, location := range []domain.Location{
		{}, {Kind: "unknown"},
		{Kind: domain.LocationFile, Line: 1}, {Kind: domain.LocationLost, Anchor: &anchor},
		{Kind: domain.LocationLine, Line: 1, StartLine: 1}, {Kind: domain.LocationLine, Line: 1, Anchor: &anchor},
		{Kind: domain.LocationLine, Line: -1},
		{Kind: domain.LocationRange, Line: 1, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 2},
		{Kind: domain.LocationRange, StartLine: 1, StartColumn: 2, EndLine: 1, EndColumn: 1},
	} {
		if err := domain.ValidateLocation(domain.ValidateLocationRequest{Location: location}); err == nil {
			t.Fatalf("accepted invalid location: %+v", location)
		}
		if _, err := domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: []domain.Thread{{ID: text, Location: location, Messages: []domain.Message{message.DTO(domain.ValueRequest{})}}}}); err == nil {
			t.Fatal("accepted invalid document")
		}
		if _, err := domain.NewThreadValue(domain.NewThreadValueRequest{ID: tid, Location: location, Messages: []domain.MessageValue{message}}); err == nil {
			t.Fatal("accepted invalid typed location")
		}
	}
}

func TestTypedValuesRejectZerosAndDetachAliases(t *testing.T) {
	text, err := domain.NewID(domain.NewIDRequest{})
	if err != nil {
		t.Fatal(err)
	}
	tid, err := domain.NewThreadID(domain.NewThreadIDRequest{Text: text})
	if err != nil {
		t.Fatal(err)
	}
	mid, err := domain.NewMessageID(domain.NewMessageIDRequest{Text: text})
	if err != nil {
		t.Fatal(err)
	}
	name := "Alice"
	discussion := "discussion"
	anchor := "YQ=="
	author := domain.Author{Kind: domain.AuthorHuman, Name: &name}
	gitlab := domain.GitLab{Kind: domain.GitLabNote, ProjectID: 1, NoteID: 1, DiscussionID: &discussion}
	message, err := domain.NewMessageValue(domain.NewMessageValueRequest{ID: mid, Author: author, Body: "body", GitLab: &gitlab})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.NewMessageValue(domain.NewMessageValueRequest{Author: author, Body: "body"}); err == nil {
		t.Fatal("accepted zero message ID")
	}
	location := domain.Location{Kind: domain.LocationRange, StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 2, Anchor: &anchor}
	messages := []domain.MessageValue{message}
	thread, err := domain.NewThreadValue(domain.NewThreadValueRequest{ID: tid, Location: location, Messages: messages})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.NewThreadValue(domain.NewThreadValueRequest{Location: location, Messages: messages}); err == nil {
		t.Fatal("accepted zero thread ID")
	}
	if _, err := domain.NewThreadValue(domain.NewThreadValueRequest{ID: tid, Location: location, Messages: []domain.MessageValue{{}}}); err == nil {
		t.Fatal("accepted zero message")
	}
	if _, err := domain.NewDocumentFromValues(domain.NewDocumentFromValuesRequest{Threads: []domain.ThreadValue{{}}}); err == nil {
		t.Fatal("accepted zero thread")
	}
	if _, err := domain.NewDocumentFromValues(domain.NewDocumentFromValuesRequest{Threads: []domain.ThreadValue{thread, thread}}); err == nil {
		t.Fatal("accepted duplicate thread")
	}
	if _, err := domain.NewThreadValue(domain.NewThreadValueRequest{ID: tid, Location: location, Messages: []domain.MessageValue{message, message}}); err == nil {
		t.Fatal("accepted duplicate message")
	}
	if _, err := domain.NewThreadValue(domain.NewThreadValueRequest{ID: tid, Location: location, Messages: make([]domain.MessageValue, domain.MaxMessagesThread+1)}); err == nil {
		t.Fatal("accepted message limit overflow")
	}
	if _, err := domain.NewDocumentFromValues(domain.NewDocumentFromValuesRequest{Threads: make([]domain.ThreadValue, domain.MaxThreads+1)}); err == nil {
		t.Fatal("accepted thread limit overflow")
	}
	threads := []domain.ThreadValue{thread}
	document, err := domain.NewDocumentFromValues(domain.NewDocumentFromValuesRequest{Threads: threads})
	if err != nil {
		t.Fatal(err)
	}
	name = "changed"
	discussion = "changed"
	anchor = "changed"
	messages[0] = domain.MessageValue{}
	threads[0] = domain.ThreadValue{}
	entries := document.Entries(domain.DocumentEntriesRequest{})
	entries[0].Messages(domain.ValueRequest{})[0] = domain.MessageValue{}
	*entries[0].Location(domain.ValueRequest{}).Anchor = "changed"
	*entries[0].Messages(domain.ValueRequest{})[0].Author(domain.ValueRequest{}).Name = "changed"
	*entries[0].Messages(domain.ValueRequest{})[0].GitLab(domain.ValueRequest{}).DiscussionID = "changed"
	dto := entries[0].DTO(domain.ValueRequest{})
	*dto.Location.Anchor = "changed"
	*dto.Messages[0].Author.Name = "changed"
	entries[0] = domain.ThreadValue{}
	actual := document.Threads(domain.DocumentThreadsRequest{})
	if *actual[0].Location.Anchor != "YQ==" || *actual[0].Messages[0].Author.Name != "Alice" || *actual[0].Messages[0].GitLab.DiscussionID != "discussion" {
		t.Fatalf("mutable aliases escaped: %+v", actual)
	}
	empty := domain.DocumentValue{}
	if len(empty.Entries(domain.DocumentEntriesRequest{})) != 0 {
		t.Fatal("zero document is not empty")
	}
}
