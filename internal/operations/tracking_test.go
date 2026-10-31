package operations_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/operations"
)

func TestTrackingDiscussionRetryRefreshAndPurge(t *testing.T) {
	tid, err := domain.NewID(domain.NewIDRequest{})
	if err != nil {
		t.Fatal(err)
	}
	mid, err := domain.NewID(domain.NewIDRequest{})
	if err != nil {
		t.Fatal(err)
	}
	location := domain.Location{Kind: domain.LocationLine, Line: 1}
	source, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte("private retained text")})
	if err != nil {
		t.Fatal(err)
	}
	capture, err := source.Capture(domain.SourceLocationRequest{Location: location})
	if err != nil {
		t.Fatal(err)
	}
	create := operations.OperationDTO{Op: domain.OpThreadCreate, ThreadID: tid, MessageID: mid, Location: location, Author: domain.Author{Kind: domain.AuthorHuman}, Body: "discussion"}
	operation, err := operations.NewValue(operations.NewValueRequest{DTO: create, Tracking: &capture})
	if err != nil {
		t.Fatal(err)
	}
	response, err := operations.Execute(operations.Request{Operation: operation})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(response.Result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), capture.SelectedText) || strings.Contains(string(raw), "selected_text") {
		t.Fatal("mutation leaked excerpts")
	}
	// Retry without capture must preserve the old evidence rather than require
	// source access. This is a pure operation, not proof of CLI retry ordering.
	operation, err = operations.NewValue(operations.NewValueRequest{DTO: create})
	if err != nil {
		t.Fatal(err)
	}
	retry, err := operations.Execute(operations.Request{Document: response.Document, Operation: operation})
	if err != nil {
		t.Fatal(err)
	}
	if retry.Changed {
		t.Fatal("retry changed state")
	}
	operation, err = operations.NewValue(operations.NewValueRequest{DTO: operations.OperationDTO{Op: domain.OpMessageUpdate, ThreadID: tid, MessageID: mid, Body: "edited discussion"}})
	if err != nil {
		t.Fatal(err)
	}
	response, err = operations.Execute(operations.Request{Document: retry.Document, Operation: operation})
	if err != nil {
		t.Fatal(err)
	}
	if response.Document.Threads(domain.DocumentThreadsRequest{})[0].Tracking.SelectedText != capture.SelectedText {
		t.Fatal("discussion discarded tracking")
	}
	fresh, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte("fresh selection")})
	if err != nil {
		t.Fatal(err)
	}
	capture, err = fresh.Capture(domain.SourceLocationRequest{Location: location})
	if err != nil {
		t.Fatal(err)
	}
	operation, err = operations.NewValue(operations.NewValueRequest{DTO: operations.OperationDTO{Op: domain.OpThreadMove, ThreadID: tid, Location: location}, Tracking: &capture})
	if err != nil {
		t.Fatal(err)
	}
	response, err = operations.Execute(operations.Request{Document: response.Document, Operation: operation})
	if err != nil {
		t.Fatal(err)
	}
	if !response.Changed || response.Document.Threads(domain.DocumentThreadsRequest{})[0].Tracking.SelectedText != capture.SelectedText {
		t.Fatal("equal coordinate move did not refresh")
	}
	for _, kind := range []string{domain.LocationFile, domain.LocationLost} {
		operation, err = operations.NewValue(operations.NewValueRequest{DTO: operations.OperationDTO{Op: domain.OpThreadMove, ThreadID: tid, Location: domain.Location{Kind: kind}}})
		if err != nil {
			t.Fatal(err)
		}
		purged, err := operations.Execute(operations.Request{Document: response.Document, Operation: operation})
		if err != nil {
			t.Fatal(err)
		}
		if purged.Document.Threads(domain.DocumentThreadsRequest{})[0].Tracking != nil {
			t.Fatal("move did not purge")
		}
	}
}
