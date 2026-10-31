package domain_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/trogers1/sidecar-comments/internal/domain"
)

func TestCaptureLogicalUTF8AndMetadataLimits(t *testing.T) {
	prefix := strings.Repeat("🙂", domain.MaxContextBytes/4+1)
	source, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(prefix + "\r\nselected\r\n" + prefix)})
	if err != nil {
		t.Fatal(err)
	}
	location := domain.Location{Kind: domain.LocationLine, Line: 2}
	record, err := source.Capture(domain.SourceLocationRequest{Location: location})
	if err != nil {
		t.Fatal(err)
	}
	if record.SelectedText != "selected" || len(record.Prefix) > domain.MaxContextBytes || len(record.Suffix) > domain.MaxContextBytes || record.PrefixAtStart || record.SuffixAtEnd {
		t.Fatalf("bad capture: %+v", record)
	}
	if record.SourceRevision != source.Fingerprint(domain.ValueRequest{}) {
		t.Fatal("revision mismatch")
	}
	empty, err := domain.NewSourceValue(domain.NewSourceValueRequest{})
	if err != nil {
		t.Fatal(err)
	}
	line, err := empty.Capture(domain.SourceLocationRequest{Location: domain.Location{Kind: domain.LocationLine, Line: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if line.SelectedText != "" || !line.PrefixAtStart || !line.SuffixAtEnd {
		t.Fatal("empty line edges")
	}
	// Every selected control byte requires six JSON bytes: valid tracking must
	// not inherit the much smaller message-metadata encoded limit.
	escaped, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(strings.Repeat("\x01", domain.MaxAnchorBytes))})
	if err != nil {
		t.Fatal(err)
	}
	record, err = escaped.Capture(domain.SourceLocationRequest{Location: domain.Location{Kind: domain.LocationLine, Line: 1}})
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(domain.ThreadMetadata{Tracking: &record})
	if err != nil {
		t.Fatal(err)
	}
	if len(metadata) <= domain.MaxMetadataBytes || len(metadata) > domain.MaxTrackingMetadataBytes {
		t.Fatal("fixture does not exercise independent bound")
	}
	if err = domain.ValidateDTO(domain.ValidateDTORequest{Bytes: metadata, Kind: "tracking_metadata"}); err != nil {
		t.Fatal(err)
	}
	record.SelectedText += "x"
	metadata, err = json.Marshal(domain.ThreadMetadata{Tracking: &record})
	if err != nil {
		t.Fatal(err)
	}
	err = domain.ValidateDTO(domain.ValidateDTORequest{Bytes: metadata, Kind: "tracking_metadata"})
	var failure *domain.Error
	if !errors.As(err, &failure) || failure.Message != "validation:tracking:too_large" {
		t.Fatalf("lost semantic limit: %v", err)
	}
}

func TestEditorRequestClosedUnion(t *testing.T) {
	id, err := domain.NewID(domain.NewIDRequest{})
	if err != nil {
		t.Fatal(err)
	}
	source := domain.HashRevision(domain.HashRevisionRequest{})
	deleted := true
	request := domain.EditorRequest{Protocol: domain.ProtocolVersion, IfRevision: domain.RevisionMissing, SourceRevision: source, Updates: []domain.EditorUpdate{{ThreadID: id, Deleted: &deleted}}}
	b, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if err = domain.ValidateDTO(domain.ValidateDTORequest{Bytes: b, Kind: "editor"}); err != nil {
		t.Fatal(err)
	}
	deleted = false
	b, err = json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if domain.ValidateDTO(domain.ValidateDTORequest{Bytes: b, Kind: "editor"}) == nil {
		t.Fatal("accepted false deletion")
	}
	deleted = true
	request.Updates[0].Location = &domain.Location{Kind: domain.LocationLine, Line: 1}
	b, err = json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if domain.ValidateDTO(domain.ValidateDTORequest{Bytes: b, Kind: "editor"}) == nil {
		t.Fatal("accepted mixed union")
	}
	request.Updates[0].Deleted = nil
	for _, kind := range []string{domain.LocationFile, domain.LocationLost, domain.LocationUnresolved} {
		request.Updates[0].Location = &domain.Location{Kind: kind}
		b, err = json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if domain.ValidateDTO(domain.ValidateDTORequest{Bytes: b, Kind: "editor"}) == nil {
			t.Fatal("accepted unattached editor location")
		}
	}
}

func TestTrackingLifecycleAndAggregate(t *testing.T) {
	source, err := domain.NewSourceValue(domain.NewSourceValueRequest{Bytes: []byte(strings.Repeat("a", domain.MaxAnchorBytes))})
	if err != nil {
		t.Fatal(err)
	}
	location := domain.Location{Kind: domain.LocationLine, Line: 1}
	record, err := source.Capture(domain.SourceLocationRequest{Location: location})
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{domain.LocationFile, domain.LocationLost} {
		if domain.ValidateThreadTracking(domain.ValidateThreadTrackingRequest{Location: domain.Location{Kind: kind}, Tracking: &record}) == nil {
			t.Fatal("accepted forbidden excerpts")
		}
	}
	if err = domain.ValidateThreadTracking(domain.ValidateThreadTrackingRequest{Location: domain.Location{Kind: domain.LocationUnresolved}, Tracking: &record}); err != nil {
		t.Fatal(err)
	}
	threads := make([]domain.Thread, domain.MaxTrackingBytes/domain.MaxAnchorBytes)
	for i := range threads {
		threads[i] = domain.Thread{Location: location, Tracking: &record}
	}
	if err = domain.ValidateDocumentTracking(domain.ValidateDocumentTrackingRequest{Threads: threads}); err != nil {
		t.Fatal(err)
	}
	threads = append(threads, threads[0])
	if domain.ValidateDocumentTracking(domain.ValidateDocumentTrackingRequest{Threads: threads}) == nil {
		t.Fatal("accepted decoded overflow")
	}
	for _, raw := range []string{`{"tracking":null}`, `{"tracking":{}}`, `{"tracking":{},"tracking":{}}`} {
		if domain.ValidateDTO(domain.ValidateDTORequest{Bytes: []byte(raw), Kind: "tracking_metadata"}) == nil {
			t.Fatal("accepted corrupt metadata")
		}
	}
	if _, err = domain.ParseLocation(domain.ParseLocationRequest{Text: "range:1:1-1:2 | YQ=="}); err == nil {
		t.Fatal("accepted legacy anchor")
	}
}
