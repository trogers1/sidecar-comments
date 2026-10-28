package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/format"
	"github.com/trogers1/sidecar-comments/internal/operations"
	"github.com/trogers1/sidecar-comments/internal/store"
)

type Request struct {
	Args           []string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
}
type config struct {
	command, output, file, source, id, messageID, thread, location, kind, name, external, bodyFile, metadata, operations, revision, lastKind string
	check, fix                                                                                                                               bool
	timeout                                                                                                                                  time.Duration
	seen                                                                                                                                     map[string]bool
}
type warning struct {
	Code string `json:"code"`
}
type assignment struct {
	ThreadID       string `json:"thread_id"`
	MessageOrdinal int    `json:"message_ordinal"`
	Line           int    `json:"line"`
	ByteOffset     int    `json:"byte_offset"`
	MessageID      string `json:"message_id"`
}
type generated struct {
	ThreadID  string `json:"thread_id"`
	MessageID string `json:"message_id"`
}
type revision struct {
	Storage   *string `json:"storage_revision"`
	Canonical *string `json:"canonical_revision"`
}
type result struct {
	operations.Result
	Previous *string `json:"previous_storage_revision,omitempty"`
	revision
	Generated   *generated `json:"generated_ids,omitempty"`
	GeneratedID string     `json:"generated_id,omitempty"`
}
type outcome struct {
	Bytes   json.RawMessage
	Changed bool
	Exit    int
	IDs     []string
}

type rawRequest[T wireDTO] struct{ Value T }

func raw[T wireDTO](r rawRequest[T]) json.RawMessage {
	v := r.Value
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'})
}

type usageRequest struct{ Field, Reason string }

func usage(r usageRequest) error {
	return &domain.Error{Code: domain.CodeUsage, Message: "usage:" + r.Field + ":" + r.Reason, Details: raw(rawRequest[usageDetails]{Value: usageDetails{Field: r.Field, Reason: r.Reason}}), Exit: domain.ExitUsage}
}

// Run never retries public output after a delivery failure. A committed mutation
// stays committed even when delivery of its result fails.
func Run(r Request) (exit int) {
	defer func(r runCleanupRequest) {
		if recover() != nil {
			exit = domain.ExitIO
		}
	}(runCleanupRequest{})
	c, err := parse(parseRequest{Args: r.Args})
	var out outcome
	if err == nil {
		out, err = execute(execRequest{Config: c, Stdin: r.Stdin})
	}
	var text string
	writer := r.Stdout
	if err != nil {
		var e *domain.Error
		if !errors.As(err, &e) {
			return domain.ExitIO
		}
		writer = r.Stderr
		exit = e.Exit
		if c.output == "json" {
			text = string(raw(rawRequest[errorEnvelope]{Value: errorEnvelope{Protocol: domain.ProtocolVersion, Error: errorValue{Code: e.Code, Message: e.Message, Details: e.Details}}})) + "\n"
		} else {
			text = fmt.Sprintf("error %s %s\n", e.Code, e.Message)
		}
	} else {
		exit = out.Exit
		switch c.output {
		case "none":
			return exit
		case "id":
			if len(out.IDs) > 0 {
				text = strings.Join(out.IDs, "\n") + "\n"
			}
		case "json":
			text = string(raw(rawRequest[successEnvelope]{Value: successEnvelope{Protocol: domain.ProtocolVersion, Command: c.command, Changed: out.Changed, Result: out.Bytes}})) + "\n"
		default:
			text = fmt.Sprintf("%s changed=%t result=%s\n", c.command, out.Changed, out.Bytes)
		}
	}
	if text != "" {
		n, err := io.WriteString(writer, text)
		if err != nil || n != len(text) {
			return domain.ExitIO
		}
	}
	return exit
}

type parseRequest struct{ Args []string }

func parse(r parseRequest) (config, error) {
	args := r.Args
	c := config{output: "text", timeout: 5 * time.Second, seen: map[string]bool{}}
	if len(args) > 0 && (args[0] == "--output" || strings.HasPrefix(args[0], "--output=")) {
		v := strings.SplitN(args[0], "=", 2)
		if len(v) == 2 {
			c.output = v[1]
			args = args[1:]
		} else {
			if len(args) < 2 {
				return c, usage(usageRequest{Field: "output", Reason: domain.UsageReasonMissing})
			}
			c.output = args[1]
			args = args[2:]
		}
	}
	if len(args) > 0 && (args[0] == "--output" || strings.HasPrefix(args[0], "--output=")) {
		return c, usage(usageRequest{Field: "output", Reason: "repeated"})
	}
	if len(args) == 0 {
		return c, usage(usageRequest{Field: "argv", Reason: domain.UsageReasonMissing})
	}
	c.command = args[0]
	if strings.Contains(c.command, "-") {
		return c, usage(usageRequest{Field: "argv", Reason: "malformed"})
	}
	args = args[1:]
	if c.command == "thread" || c.command == "message" {
		if len(args) == 0 {
			return c, usage(usageRequest{Field: "argv", Reason: domain.UsageReasonMissing})
		}
		c.command += "-" + args[0]
		args = args[1:]
	}
	definition, ok := commandDefinitions[c.command]
	flags := definition.allowed
	if !ok {
		return c, usage(usageRequest{Field: "argv", Reason: "malformed"})
	}
	mutation := isMutation(mutationRequest{Command: c.command})
	if c.output != "text" && c.output != "json" && c.output != "none" && c.output != "id" {
		return c, usage(usageRequest{Field: "output", Reason: "malformed"})
	}
	if c.output == "none" && !mutation || c.output == "id" && c.command != "thread-create" && c.command != "message-add" && c.command != "fix" {
		return c, usage(usageRequest{Field: "output", Reason: "incompatible"})
	}
	fs := flag.NewFlagSet(c.command, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	values := map[string]*string{"file": &c.file, "source": &c.source, "id": &c.id, "message-id": &c.messageID, "thread": &c.thread, "location": &c.location, "author-kind": &c.kind, "author-name": &c.name, "external-author-id": &c.external, "body-file": &c.bodyFile, "gitlab-json": &c.metadata, "operations-file": &c.operations, "if-revision": &c.revision, "if-last-author-kind": &c.lastKind}
	if mutation {
		flags += " if-revision lock-timeout"
	}
	for _, name := range strings.Fields(flags) {
		switch name {
		case "check":
			fs.BoolVar(&c.check, name, false, "")
		case "add-missing-message-ids":
			fs.BoolVar(&c.fix, name, false, "")
		case "lock-timeout":
			fs.DurationVar(&c.timeout, name, 5*time.Second, "")
		default:
			fs.StringVar(values[name], name, "", "")
		}
	}
	ordinary := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			return c, usage(usageRequest{Field: "argv", Reason: "malformed"})
		}
		name := strings.SplitN(strings.TrimLeft(a, "-"), "=", 2)[0]
		if c.seen[name] {
			return c, usage(usageRequest{Field: "argv", Reason: "repeated"})
		}
		c.seen[name] = true
		if name == "if-revision" || name == "lock-timeout" {
			if ordinary {
				return c, usage(usageRequest{Field: "argv", Reason: "malformed"})
			}
		} else {
			ordinary = true
		}
		f := fs.Lookup(name)
		if f == nil {
			return c, usage(usageRequest{Field: "argv", Reason: "malformed"})
		}
		if !strings.Contains(a, "=") && name != "check" && name != "add-missing-message-ids" {
			i++
			if i >= len(args) {
				return c, usage(usageRequest{Field: "argv", Reason: domain.UsageReasonMissing})
			}
		}
	}
	if fs.Parse(args) != nil || fs.NArg() != 0 {
		return c, usage(usageRequest{Field: "argv", Reason: "malformed"})
	}
	if c.command != "version" && c.file == "" {
		return c, usage(usageRequest{Field: "argv", Reason: "required"})
	}
	if c.timeout < time.Millisecond || c.timeout > 60*time.Second {
		return c, usage(usageRequest{Field: "argv", Reason: "malformed"})
	}
	if c.seen["if-revision"] && c.revision != domain.RevisionMissing && domain.ValidateRevision(domain.ValidateRevisionRequest{Text: c.revision}) != nil {
		return c, usage(usageRequest{Field: "revision", Reason: "malformed"})
	}
	for _, n := range strings.Fields(definition.required) {
		if *values[n] == "" {
			return c, usage(usageRequest{Field: "argv", Reason: "required"})
		}
	}
	for _, name := range []string{"id", "message-id", "thread"} {
		if c.seen[name] && domain.ValidateID(domain.ValidateIDRequest{ID: *values[name]}) != nil {
			return c, usage(usageRequest{Field: "argv", Reason: "malformed"})
		}
	}
	if c.command == "thread-create" && c.seen["id"] != c.seen["message-id"] {
		return c, usage(usageRequest{Field: "argv", Reason: "incompatible"})
	}
	if c.command == "fix" && !c.fix {
		return c, usage(usageRequest{Field: "argv", Reason: "required"})
	}
	for _, name := range []string{"file", "source", "id", "message-id", "thread", "location", "author-kind", "author-name", "external-author-id", "body-file", "gitlab-json", "operations-file", "if-revision", "if-last-author-kind"} {
		if c.seen[name] && *values[name] == "" {
			field := "argv"
			switch name {
			case "if-revision":
				field = "revision"
			case "body-file":
				field = "body"
			case "gitlab-json":
				field = "metadata"
			case "operations-file":
				field = "operations"
			}
			return c, usage(usageRequest{Field: field, Reason: "malformed"})
		}
	}
	count := 0
	for _, p := range []string{c.bodyFile, c.metadata, c.operations} {
		if p == "-" {
			count++
		}
	}
	if count > 1 {
		return c, usage(usageRequest{Field: "stdin", Reason: "stdin_multiple"})
	}
	return c, nil
}

type mutationRequest struct{ Command string }

func isMutation(r mutationRequest) bool {
	return commandDefinitions[r.Command].mutation
}

type inputRequest struct {
	Path, Field string
	Limit       int64
	Stdin       io.Reader
}

func input(r inputRequest) (data []byte, err error) {
	var reader io.Reader = r.Stdin
	if r.Path != "-" {
		f, e := os.Open(r.Path)
		if e != nil {
			return nil, usage(usageRequest{Field: r.Field, Reason: "malformed"})
		}
		defer func(cleanup inputCleanupRequest) {
			if e := f.Close(); err == nil && e != nil {
				err = usage(usageRequest{Field: r.Field, Reason: "malformed"})
			}
		}(inputCleanupRequest{})
		reader = f
	}
	b, e := io.ReadAll(io.LimitReader(reader, r.Limit+1))
	if e != nil {
		return nil, usage(usageRequest{Field: r.Field, Reason: "malformed"})
	}
	if int64(len(b)) > r.Limit {
		return nil, usage(usageRequest{Field: r.Field, Reason: "too_large"})
	}
	return b, nil
}

type hashRequest struct{ Bytes []byte }

func hash(r hashRequest) string {
	return domain.HashRevision(domain.HashRevisionRequest{Bytes: r.Bytes})
}

type sourceRequest struct{}
type transactionCleanupRequest struct{}

type execRequest struct {
	Config config
	Stdin  io.Reader
}

func execute(r execRequest) (out outcome, err error) {
	c := r.Config
	if c.command == "version" {
		out.Bytes = raw(rawRequest[versionResult]{Value: versionResult{CLIVersion: "1.0.0", FormatVersion: domain.FormatVersion, Protocol: domain.ProtocolVersion}})
		return
	}
	op := operations.OperationDTO{Op: commandDefinitions[c.command].operation, ThreadID: c.id, MessageID: c.messageID, IfLastAuthorKind: c.lastKind}
	if strings.HasPrefix(c.command, "message-") {
		op.ThreadID = c.thread
		op.MessageID = c.id
	}
	if c.location != "" {
		op.Location, err = domain.ParseLocation(domain.ParseLocationRequest{Text: c.location})
		if err != nil {
			return out, usage(usageRequest{Field: "argv", Reason: "malformed"})
		}
	}
	if c.kind != "" {
		op.Author.Kind = c.kind
		if c.seen["author-name"] {
			op.Author.Name = &c.name
		}
		if c.kind == domain.AuthorExternal {
			if !c.seen["author-name"] || !c.seen["external-author-id"] {
				return out, usage(usageRequest{Field: "argv", Reason: "required"})
			}
			op.Author.Name = nil
			op.Author.ExternalAuthor = &domain.ExternalAuthor{ID: c.external, Name: c.name}
		} else if c.seen["external-author-id"] {
			return out, usage(usageRequest{Field: "argv", Reason: "incompatible"})
		}

	}
	if c.bodyFile != "" {
		b, e := input(inputRequest{Path: c.bodyFile, Field: "body", Limit: 2 * (domain.MaxBodyBytes + 1), Stdin: r.Stdin})
		if e != nil {
			return out, e
		}
		if bytes.HasPrefix(b, []byte{239, 187, 191}) || !utf8.Valid(b) || bytes.IndexByte(b, 0) >= 0 {
			return out, usage(usageRequest{Field: "body", Reason: "malformed"})
		}
		withoutCRLF := bytes.ReplaceAll(b, []byte("\r\n"), nil)
		if bytes.Contains(b, []byte("\r\n")) && bytes.Contains(withoutCRLF, []byte("\n")) {
			return out, usage(usageRequest{Field: "body", Reason: "malformed"})
		}
		b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
		if bytes.IndexByte(b, '\r') >= 0 || len(b) > domain.MaxBodyBytes+1 {
			return out, usage(usageRequest{Field: "body", Reason: "malformed"})
		}
		op.Body = strings.TrimSuffix(string(b), "\n")

	}
	if c.metadata != "" {
		b, e := input(inputRequest{Path: c.metadata, Field: "metadata", Limit: domain.MaxMetadataBytes, Stdin: r.Stdin})
		if e != nil {
			return out, e
		}
		if domain.ValidateDTO(domain.ValidateDTORequest{Bytes: b, Kind: "gitlab"}) != nil {
			return out, usage(usageRequest{Field: "metadata", Reason: "malformed"})
		}
		var g domain.GitLab
		d := json.NewDecoder(bytes.NewReader(b))
		d.DisallowUnknownFields()
		if d.Decode(&g) != nil {
			return out, usage(usageRequest{Field: "metadata", Reason: "malformed"})
		}
		op.GitLab = &g

	}
	var batch applyRequest

	if c.command == "apply" {
		b, e := input(inputRequest{Path: c.operations, Field: "operations", Limit: domain.MaxApplyBytes, Stdin: r.Stdin})
		if e != nil {
			return out, e
		}
		batch, e = decodeApply(applyDecodeRequest{Bytes: b})
		if e != nil {
			return out, e
		}
		if batch.IfRevision != "" {
			if c.revision != "" && c.revision != batch.IfRevision {
				return out, usage(usageRequest{Field: "revision", Reason: "incompatible"})
			}
			c.revision = batch.IfRevision
		}
	}
	if commandDefinitions[c.command].operation != "" {
		if e := operations.ValidateOperationFields(operations.ValidateOperationFieldsRequest{DTO: op, AllowMissingIDs: true}); e != nil {
			field := "argv"
			var invalid *operations.FieldError
			if errors.As(e, &invalid) {
				field = invalid.Field
			}
			return out, usage(usageRequest{Field: field, Reason: "malformed"})
		}
	}
	if isMutation(mutationRequest{Command: c.command}) {
		if err = store.CheckWriterCapability(store.CheckWriterCapabilityRequest{}); err != nil {
			return
		}
	}
	locations := []domain.Location{}
	if c.command == "thread-create" || c.command == "thread-move" {
		locations = append(locations, op.Location)
	}
	for _, o := range batch.Operations {
		if o.Op(operations.ValueRequest{}) == domain.OpThreadCreate || o.Op(operations.ValueRequest{}) == domain.OpThreadMove {
			locations = append(locations, o.Location(operations.ValueRequest{}))
		}
	}
	source := func(r sourceRequest) error {
		if len(locations) == 0 {
			return nil
		}
		return store.ValidateSource(store.ValidateSourceRequest{Path: strings.TrimSuffix(c.file, ".comments"), Locations: locations})
	}
	snap, e := store.Read(store.ReadRequest{Path: c.file, Kind: store.ReadTarget})
	if e != nil {
		return out, e
	}
	if err = source(sourceRequest{}); err != nil {
		return
	}
	var tx *store.Transaction
	if isMutation(mutationRequest{Command: c.command}) && !c.check {
		tx, err = store.Begin(store.BeginRequest{Path: c.file, Timeout: c.timeout})
		if err != nil {
			return
		}
		defer func(r transactionCleanupRequest) {
			if e := tx.Close(store.TransactionCloseRequest{}); err == nil && e != nil {
				err = e
			}
		}(transactionCleanupRequest{})
		snap, err = tx.Read(store.TransactionReadRequest{})
		if err != nil {
			return
		}
	}
	if c.revision != "" && ((c.revision == domain.RevisionMissing && snap.Exists) || (c.revision != domain.RevisionMissing && (snap.Revision == nil || *snap.Revision != c.revision))) {
		return out, domain.RevisionConflict(domain.RevisionConflictRequest{Expected: c.revision, CurrentStorageRevision: snap.Revision})
	}
	mode := "normal"
	if c.command == "format" {
		mode = "format"
	}
	if c.command == "fix" || c.command == "validate" {
		mode = "repair"
	}
	doc, e := format.Parse(format.ParseRequest{Bytes: snap.Bytes, Mode: mode})
	if e != nil {
		return out, e
	}
	if err = source(sourceRequest{}); err != nil {
		return
	}
	var repairThreads []domain.Thread
	diagnostics := []domain.Diagnostic{}
	if doc.Repair != nil {
		repairThreads = doc.Repair.Threads
		diagnostics = doc.Repair.Diagnostics
	}
	var canonical []byte
	if doc.Repair == nil {
		canonical = format.Encode(format.EncodeRequest{Document: doc.Value})
	}
	cr := hash(hashRequest{Bytes: canonical})
	rev := revision{Storage: snap.Revision, Canonical: &cr}
	warnings := []warning{}
	if snap.Exists && len(snap.Bytes) == 0 {
		warnings = append(warnings, warning{Code: "noncanonical_empty"})
	}
	if len(diagnostics) > 0 && c.command != "fix" && c.command != "validate" {
		return out, &domain.Error{Code: domain.CodeRepair, Message: "repair:document", Exit: domain.ExitValidation, Details: raw(rawRequest[domain.RepairDetails]{Value: domain.RepairDetails{Diagnostics: diagnostics}})}
	}
	switch c.command {
	case "validate":
		if c.source != "" {
			ls := []domain.Location{}
			for _, t := range doc.Value.Entries(domain.DocumentEntriesRequest{}) {
				ls = append(ls, t.Location(domain.ValueRequest{}))
			}
			for _, t := range repairThreads {
				ls = append(ls, t.Location)
			}
			if err = store.ValidateSource(store.ValidateSourceRequest{Path: c.source, Locations: ls}); err != nil {
				return
			}
		}
		if len(diagnostics) > 0 {
			out.Exit = domain.ExitValidation
			out.Bytes = raw(rawRequest[invalidResult]{Value: invalidResult{Valid: false, Repair: repairDetails{Diagnostics: diagnostics}, revision: revision{Storage: snap.Revision, Canonical: nil}}})
		} else {
			out.Bytes = raw(rawRequest[validResult]{Value: validResult{Valid: true, Warnings: warnings, revision: rev}})
		}
		return
	case "thread-list":
		out.Bytes = raw(rawRequest[listResult]{Value: listResult{Threads: doc.Value.Threads(domain.DocumentThreadsRequest{}), Warnings: warnings, revision: rev}})
		return
	case "thread-get":
		id, e := domain.NewThreadID(domain.NewThreadIDRequest{Text: c.id})
		if e != nil {
			return out, e
		}
		for _, t := range doc.Value.Entries(domain.DocumentEntriesRequest{}) {
			if t.ID(domain.ValueRequest{}) == id {
				dto := t.DTO(domain.ValueRequest{})
				out.Bytes = raw(rawRequest[result]{Value: result{Result: operations.Result{Thread: &dto}, revision: rev}})
				return
			}
		}
		return out, &domain.Error{Code: domain.CodeNotFound, Message: "not_found:thread", Exit: domain.ExitNotFound, Details: raw(rawRequest[notFoundDetails]{Value: notFoundDetails{Entity: "thread", ID: c.id}})}
	}
	assignments := []assignment{}
	res := result{}
	opResults := []operationResult{}
	if c.command == "fix" {
		if len(diagnostics) > 0 && c.check {
			out.Exit = domain.ExitValidation
			out.Bytes = raw(rawRequest[assignmentsResult]{Value: assignmentsResult{Assignments: assignments, revision: revision{Storage: snap.Revision, Canonical: nil}}})
			return
		}
		for _, d := range diagnostics {
			id, e := uniqueID(uniqueIDRequest{RepairThreads: repairThreads, Namespace: "message"})
			if e != nil {
				return out, e
			}
			for i := range repairThreads {
				if repairThreads[i].ID == d.ThreadID {
					repairThreads[i].Messages[d.MessageOrdinal-1].ID = id
				}
			}
			assignments = append(assignments, assignment{ThreadID: d.ThreadID, MessageOrdinal: d.MessageOrdinal, Line: d.Line, ByteOffset: d.ByteOffset, MessageID: id})
			out.IDs = append(out.IDs, id)
		}
		out.Changed = len(assignments) > 0
	} else if c.command != "format" {
		if c.command == "thread-create" && op.ThreadID == "" {
			op.ThreadID, err = uniqueID(uniqueIDRequest{Document: doc.Value, Namespace: "thread"})
			if err != nil {
				return
			}
			op.MessageID, err = uniqueID(uniqueIDRequest{Document: doc.Value, Namespace: "message"})
			if err != nil {
				return
			}
			res.Generated = &generated{ThreadID: op.ThreadID, MessageID: op.MessageID}
			out.IDs = []string{op.ThreadID, op.MessageID}
		}
		if c.command == "message-add" && op.MessageID == "" {
			op.MessageID, err = uniqueID(uniqueIDRequest{Document: doc.Value, Namespace: "message"})
			if err != nil {
				return
			}
			res.GeneratedID = op.MessageID
			out.IDs = []string{op.MessageID}
		}
		ops := batch.Operations
		if c.command != "apply" {
			value, e := operations.NewValue(operations.NewValueRequest{DTO: op})
			if e != nil {
				return out, e
			}
			ops = []operations.Value{value}
		}
		for i, o := range ops {
			var index *int
			if c.command == "apply" {
				index = &i
			}
			response, e := operations.Execute(operations.Request{Document: doc.Value, Operation: o, Index: index})
			if e != nil {
				if c.command != "apply" {
					e = commandError(commandErrorRequest{Error: e, Command: c.command})
				}
				return out, e
			}
			doc.Value = response.Document

			if len(format.Encode(format.EncodeRequest{Document: doc.Value})) > domain.MaxTargetBytes {
				return out, domain.Validation(domain.ValidationRequest{Path: "target_size", Reason: "too_large"})
			}
			out.Changed = out.Changed || response.Changed
			res.Result = response.Result
			opResults = append(opResults, operationResult{Index: i, Op: o.Op(operations.ValueRequest{}), Changed: response.Changed, Result: response.Result})
		}
	}
	if c.command == "fix" && doc.Repair != nil {
		value, e := domain.NewDocumentValue(domain.NewDocumentValueRequest{Threads: repairThreads})
		if e != nil {
			return out, e
		}
		doc.Value = value

	}
	canonical = format.Encode(format.EncodeRequest{Document: doc.Value})
	different := !bytes.Equal(canonical, snap.Bytes) || snap.Exists && len(canonical) == 0
	if c.check {
		out.Changed = false
		if different {
			out.Exit = domain.ExitValidation
		}
	} else if different {
		if err = tx.Commit(store.CommitRequest{Bytes: canonical}); err != nil {
			return
		}
		out.Changed = true
	}
	cr = hash(hashRequest{Bytes: canonical})
	rev = revision{Storage: snap.Revision, Canonical: &cr}
	if !c.check && different {
		if len(canonical) == 0 {
			rev.Storage = nil
		} else {
			v := hash(hashRequest{Bytes: canonical})
			rev.Storage = &v
		}
	}
	switch c.command {
	case "format":
		out.Bytes = raw(rawRequest[formatResult]{Value: formatResult{Formatted: out.Changed, revision: rev}})
	case "fix":
		out.Bytes = raw(rawRequest[assignmentsResult]{Value: assignmentsResult{Assignments: assignments, revision: rev}})
	case "apply":
		out.Bytes = raw(rawRequest[applyResult]{Value: applyResult{Operations: opResults, revision: rev}})
	default:
		res.revision = rev
		if c.command == "thread-delete" || c.command == "message-delete" && rev.Storage == nil {
			res.Previous = snap.Revision
		}
		out.Bytes = raw(rawRequest[result]{Value: res})
	}
	return
}

type applyDTO struct {
	Protocol   int                       `json:"protocol"`
	IfRevision string                    `json:"if_revision,omitempty"`
	Operations []operations.OperationDTO `json:"operations"`
}
type applyRequest struct {
	IfRevision string
	Operations []operations.Value
}
type operationResult struct {
	Index   int               `json:"index"`
	Op      string            `json:"op"`
	Changed bool              `json:"changed"`
	Result  operations.Result `json:"result"`
}

type applyDecodeRequest struct{ Bytes []byte }

func decodeApply(r applyDecodeRequest) (applyRequest, error) {
	b := r.Bytes
	var a applyRequest
	var dto applyDTO
	if domain.ValidateDTO(domain.ValidateDTORequest{Bytes: b, Kind: "apply"}) != nil {
		return a, usage(usageRequest{Field: "operations", Reason: "malformed"})
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&dto) != nil {
		return a, usage(usageRequest{Field: "operations", Reason: "malformed"})
	}
	if dto.Protocol != domain.ProtocolVersion {
		return a, &domain.Error{Code: domain.CodeUnsupported, Message: "unsupported:protocol", Exit: domain.ExitUnsupported, Details: raw(rawRequest[unsupportedDetails]{Value: unsupportedDetails{Received: "protocol"}})}
	}
	a.IfRevision = dto.IfRevision
	for _, o := range dto.Operations {
		value, err := operations.NewValue(operations.NewValueRequest{DTO: o})
		if err != nil {
			return a, usage(usageRequest{Field: "operations", Reason: "malformed"})
		}
		a.Operations = append(a.Operations, value)
	}
	return a, nil
}
