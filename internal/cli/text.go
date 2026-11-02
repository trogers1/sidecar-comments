package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/trogers1/sidecar-comments/internal/domain"
	"github.com/trogers1/sidecar-comments/internal/operations"
)

type textRequest struct {
	Config  config
	Outcome outcome
}
type textDecodeRequest[T wireDTO] struct {
	Bytes json.RawMessage
	Value *T
}

func decodeText[T wireDTO](r textDecodeRequest[T]) {
	if err := json.Unmarshal(r.Bytes, r.Value); err != nil {
		panic(fmt.Errorf("decode CLI result: %w", err))
	}
}
func humanText(r textRequest) string {
	c, out := r.Config, r.Outcome
	var b strings.Builder
	switch c.command {
	case "version":
		var v versionResult
		decodeText(textDecodeRequest[versionResult]{out.Bytes, &v})
		fmt.Fprintf(&b, "Sidecar %s\nFormat version: %d\nProtocol version: %d\n", v.CLIVersion, v.FormatVersion, v.Protocol)
	case "source-inspect":
		var v sourceInspectResult
		decodeText(textDecodeRequest[sourceInspectResult]{out.Bytes, &v})
		fmt.Fprintf(&b, "Source revision: %s\nLogical lines: %d\nLogical UTF-8 bytes: %d\n", v.SourceRevision, v.LineCount, v.ByteCount)
	case "validate":
		if out.Exit != 0 {
			var v invalidResult
			decodeText(textDecodeRequest[invalidResult]{out.Bytes, &v})
			fmt.Fprintf(&b, "Validation failed: %q\n", c.file)
			for _, d := range v.Repair.Diagnostics {
				fmt.Fprintf(&b, "  %s: thread %s, message %d, line %d (byte %d)\n", readable(readableRequest{d.Code}), d.ThreadID, d.MessageOrdinal, d.Line, d.ByteOffset)
			}
			writeFileSuggestion(fileSuggestionRequest{Writer: &b, Command: "fix", File: c.file, Prefix: "Repair with: ", Ending: "\n"})
		} else {
			var v validResult
			decodeText(textDecodeRequest[validResult]{out.Bytes, &v})
			fmt.Fprintf(&b, "Validation passed: %q\n", c.file)
			writeWarnings(warningsTextRequest{&b, v.Warnings})
		}
	case "thread-list":
		var v listResult
		decodeText(textDecodeRequest[listResult]{out.Bytes, &v})
		fmt.Fprintf(&b, "%d thread(s) in %q\n", len(v.Threads), c.file)
		for _, t := range v.Threads {
			writeThread(threadTextRequest{Writer: &b, ID: t.ID, Location: t.Location, Messages: t.Messages})
		}
		writeWarnings(warningsTextRequest{&b, v.Warnings})
	case "sync":
		var v syncResult
		decodeText(textDecodeRequest[syncResult]{out.Bytes, &v})
		if v.Check {
			fmt.Fprintf(&b, "Sync check: %q (would change: %t).\n", c.file, v.WouldChange)
		} else {
			fmt.Fprintf(&b, "Sync: %q (%s).\n", c.file, changeText(changeTextRequest{Changed: out.Changed}))
		}
		writeTrackingOutcomes(trackingOutcomesTextRequest{Writer: &b, Outcomes: v.Outcomes})
	case "editor-update":
		var v editorResult
		decodeText(textDecodeRequest[editorResult]{out.Bytes, &v})
		fmt.Fprintf(&b, "Editor update: %q (%s).\n", c.file, changeText(changeTextRequest{Changed: out.Changed}))
		writeTrackingOutcomes(trackingOutcomesTextRequest{Writer: &b, Outcomes: v.Outcomes})
	case "format":
		if c.check {
			writeCheck(checkTextRequest{&b, c, out})
		} else if out.Changed {
			fmt.Fprintf(&b, "Formatted %q\n", c.file)
		} else {
			fmt.Fprintf(&b, "Already formatted: %q\n", c.file)
		}
	case "fix":
		var v assignmentsResult
		decodeText(textDecodeRequest[assignmentsResult]{out.Bytes, &v})
		if c.check {
			writeCheck(checkTextRequest{&b, c, out})
		} else {
			fmt.Fprintf(&b, "Repair complete: %q\nAssigned %d missing message ID(s).\n", c.file, len(v.Assignments))
		}
		for _, a := range v.Assignments {
			fmt.Fprintf(&b, "  Thread %s, message %d (line %d): %s\n", a.ThreadID, a.MessageOrdinal, a.Line, a.MessageID)
		}
	case "apply":
		var v applyResult
		decodeText(textDecodeRequest[applyResult]{out.Bytes, &v})
		fmt.Fprintf(&b, "Applied %d operation(s) to %q (%s).\n", len(v.Operations), c.file, changeText(changeTextRequest{out.Changed}))
		for _, op := range v.Operations {
			fmt.Fprintf(&b, "  %d. %s: %s\n", op.Index+1, readable(readableRequest{op.Op}), changeText(changeTextRequest{op.Changed}))
			writeResult(resultTextRequest{&b, op.Result})
		}
	default:
		var v result
		decodeText(textDecodeRequest[result]{out.Bytes, &v})
		if c.command != "thread-get" {
			fmt.Fprintf(&b, "%s: %q (%s).\n", readable(readableRequest{c.command}), c.file, changeText(changeTextRequest{out.Changed}))
		}
		writeResult(resultTextRequest{&b, v.Result})
	}
	return b.String()
}

type trackingOutcomesTextRequest struct {
	Writer   *strings.Builder
	Outcomes []domain.TrackingOutcome
}

func writeTrackingOutcomes(r trackingOutcomesTextRequest) {
	for _, o := range r.Outcomes {
		fmt.Fprintf(r.Writer, "  Thread %s: %s (%s; %s) — %s\n", o.ThreadID, o.Outcome, o.Reason, o.Evidence, domain.LocationText(domain.LocationTextRequest{Location: o.Location}))
	}
}

type changeTextRequest struct{ Changed bool }

func changeText(r changeTextRequest) string {
	if r.Changed {
		return "changed"
	}
	return "no changes"
}

type checkTextRequest struct {
	Writer  *strings.Builder
	Config  config
	Outcome outcome
}

func writeCheck(r checkTextRequest) {
	if r.Outcome.Exit == 0 {
		fmt.Fprintf(r.Writer, "Check passed: %q needs no %s changes.\n", r.Config.file, r.Config.command)
	} else {
		fmt.Fprintf(r.Writer, "Check failed: %q needs %s changes.\n", r.Config.file, r.Config.command)
		writeFileSuggestion(fileSuggestionRequest{Writer: r.Writer, Command: r.Config.command, File: r.Config.file, Prefix: "Run ", Ending: " to write them.\n"})
	}
}

type shellArgumentRequest struct{ Text string }

// POSIX single quotes preserve literal bytes, including shell metacharacters.
// Controls must never enter a copyable terminal command.
func shellArgument(r shellArgumentRequest) string {
	if strings.ContainsFunc(r.Text, unicode.IsControl) {
		panic("cannot suggest a shell argument containing control characters")
	}
	return "'" + strings.ReplaceAll(r.Text, "'", "'\\''") + "'"
}

type fileSuggestionRequest struct {
	Writer                        *strings.Builder
	Command, File, Prefix, Ending string
}

func writeFileSuggestion(r fileSuggestionRequest) {
	if strings.ContainsFunc(r.File, unicode.IsControl) {
		fmt.Fprintf(r.Writer, "Use the %s command with the exact sidecar path", r.Command)
		if r.Command == "fix" {
			r.Writer.WriteString(" and --add-missing-message-ids")
		}
		r.Writer.WriteString("; no copyable command is shown because the path contains control characters.\n")
		return
	}
	fmt.Fprintf(r.Writer, "%ssidecar %s --file %s", r.Prefix, r.Command, shellArgument(shellArgumentRequest{Text: r.File}))
	if r.Command == "fix" {
		r.Writer.WriteString(" --add-missing-message-ids")
	}
	r.Writer.WriteString(r.Ending)
}

type warningsTextRequest struct {
	Writer   *strings.Builder
	Warnings []warning
}

func writeWarnings(r warningsTextRequest) {
	for _, w := range r.Warnings {
		fmt.Fprintf(r.Writer, "Warning: %s. Run sidecar format --file PATH to canonicalize the sidecar.\n", readable(readableRequest{w.Code}))
	}
}

type resultTextRequest struct {
	Writer *strings.Builder
	Result operations.Result
}

func writeResult(r resultTextRequest) {
	if r.Result.Thread != nil {
		t := r.Result.Thread
		writeThread(threadTextRequest{Writer: r.Writer, ID: t.ID, Location: t.Location, Messages: t.Messages})
	}
	// Message add/update also return the containing thread, which already
	// renders the message.
	if r.Result.Message != nil && r.Result.Thread == nil {
		writeMessage(messageTextRequest{r.Writer, *r.Result.Message})
	}
	if r.Result.DeletedThreadID != "" {
		fmt.Fprintf(r.Writer, "Deleted thread: %s\n", r.Result.DeletedThreadID)
	}
	if r.Result.DeletedMessageID != "" {
		fmt.Fprintf(r.Writer, "Deleted message: %s\n", r.Result.DeletedMessageID)
	}
}

type threadTextRequest struct {
	Writer   *strings.Builder
	ID       string
	Location domain.Location
	Messages []domain.Message
}

func writeThread(r threadTextRequest) {
	l := r.Location
	location := l.Kind
	switch l.Kind {
	case domain.LocationLine:
		location = fmt.Sprintf("line %d", l.Line)
	case domain.LocationRange:
		location = fmt.Sprintf("range %d:%d–%d:%d", l.StartLine, l.StartColumn, l.EndLine, l.EndColumn)
	}
	fmt.Fprintf(r.Writer, "\nThread %s — %s\n", r.ID, location)
	for _, m := range r.Messages {
		writeMessage(messageTextRequest{r.Writer, m})
	}
}

type messageTextRequest struct {
	Writer  *strings.Builder
	Message domain.Message
}

func writeMessage(r messageTextRequest) {
	m := r.Message
	author := m.Author.Kind
	if m.Author.Name != nil {
		author = *m.Author.Name + " (" + m.Author.Kind + ")"
	}
	if m.Author.ExternalAuthor != nil {
		author = m.Author.ExternalAuthor.Name + " (external: " + m.Author.ExternalAuthor.ID + ")"
	}
	fmt.Fprintf(r.Writer, "  Message %s — %s\n", m.ID, author)
	for _, line := range strings.Split(terminalText(terminalTextRequest{Text: m.Body}), "\n") {
		fmt.Fprintf(r.Writer, "    %s\n", line)
	}
}

type terminalTextRequest struct{ Text string }

// Escape terminal controls only at the human presentation boundary. Newlines
// remain structural; stored bodies and machine output retain their exact bytes.
func terminalText(r terminalTextRequest) string {
	var b strings.Builder
	for _, ch := range r.Text {
		switch {
		case ch == '\n':
			b.WriteRune(ch)
		case ch < 0x20 || ch == 0x7f:
			fmt.Fprintf(&b, "\\x%02x", ch)
		case ch >= 0x80 && ch <= 0x9f:
			fmt.Fprintf(&b, "\\u%04x", ch)
		default:
			b.WriteRune(ch)
		}
	}
	return b.String()
}

type readableRequest struct{ Text string }

func readable(r readableRequest) string {
	return strings.NewReplacer("_", " ", "-", " ", ":", ": ").Replace(r.Text)
}

type humanErrorRequest struct {
	Config config
	Error  *domain.Error
}

func humanError(r humanErrorRequest) string {
	e, c := r.Error, r.Config
	explanation := c.diagnostic
	next := "Check the input and try again."
	if explanation == "" {
		switch e.Code {
		case domain.CodeUsage:
			var d usageDetails
			decodeText(textDecodeRequest[usageDetails]{e.Details, &d})
			reason := readable(readableRequest{d.Reason})
			switch d.Reason {
			case "missing":
				reason = "a value is missing"
			case "required":
				reason = "a required flag is missing"
			case "malformed":
				reason = "the value or flag syntax is invalid"
			case "incompatible":
				reason = "these options cannot be used together"
			case "repeated":
				reason = "a flag was supplied more than once"
			case "stdin_multiple":
				reason = "only one input may read from stdin"
			case "too_large":
				reason = "the input exceeds the size limit"
			}
			field := d.Field
			if field == "output" {
				field = "--output"
			}
			explanation = fmt.Sprintf("Invalid %s: %s.", field, reason)
		case domain.CodeNotFound:
			var d notFoundDetails
			decodeText(textDecodeRequest[notFoundDetails]{e.Details, &d})
			explanation = fmt.Sprintf("The requested %s %q was not found.", d.Entity, d.ID)
			next = "List threads to find an existing ID."
		case domain.CodeValidation:
			var d domain.ValidationDetails
			decodeText(textDecodeRequest[domain.ValidationDetails]{e.Details, &d})
			explanation, next = validationText(validationTextRequest{Details: d, File: c.file})
		case domain.CodeConflict:
			var d domain.ConflictDetails
			decodeText(textDecodeRequest[domain.ConflictDetails]{e.Details, &d})
			explanation, next = conflictText(conflictTextRequest{Details: d})
		case domain.CodePrecondition:
			var d operations.PreconditionDetails
			decodeText(textDecodeRequest[operations.PreconditionDetails]{e.Details, &d})
			explanation, next = preconditionText(preconditionTextRequest{Details: d})
		case domain.CodeIO:
			var d domain.IODetails
			decodeText(textDecodeRequest[domain.IODetails]{e.Details, &d})
			explanation, next = ioText(ioTextRequest{Details: d})
		case domain.CodeUnsupported:
			var d unsupportedDetails
			decodeText(textDecodeRequest[unsupportedDetails]{e.Details, &d})
			switch d.Received {
			case "format":
				explanation = fmt.Sprintf("This sidecar is not in format %d, the only format this version reads. Other format versions are not migrated.", domain.FormatVersion)
				next = "Use the Sidecar release that wrote this file, or recreate the comments with this version."
			case "protocol":
				explanation = fmt.Sprintf("The request uses an unsupported protocol version; this version speaks protocol %d.", domain.ProtocolVersion)
				next = fmt.Sprintf("Set the request's protocol field to %d.", domain.ProtocolVersion)
			default:
				explanation = upperFirst(upperFirstRequest{Text: readable(readableRequest{d.Received})}) + " is not supported."
				next = "Check the Sidecar version and the input you supplied."
			}
		case domain.CodeRepair:
			explanation = "The sidecar has missing message IDs and needs repair."
			next = "Run sidecar fix --file PATH --add-missing-message-ids."
		default:
			explanation = readable(readableRequest{e.Message}) + "."
		}
	}
	hint := "sidecar --help"
	if _, ok := commandDefinitions[c.command]; ok {
		hint = "sidecar " + commandWords(commandWordsRequest{Command: c.command}) + " --help"
	}
	target := ""
	if c.file != "" {
		target = fmt.Sprintf("Sidecar: %q\n", c.file)
	}
	return fmt.Sprintf("error %s: %s\n%s%s\nFor usage, run: %s\n", e.Code, explanation, target, next, hint)
}

// Human limits are stated in whole MiB; these fail to compile otherwise.
var (
	_ = [1]struct{}{}[domain.MaxSourceBytes%(1<<20)]
	_ = [1]struct{}{}[domain.MaxTargetBytes%(1<<20)]
	_ = [1]struct{}{}[domain.MaxTrackingBytes%(1<<20)]
)

type upperFirstRequest struct{ Text string }

func upperFirst(r upperFirstRequest) string {
	if r.Text == "" {
		return r.Text
	}
	return strings.ToUpper(r.Text[:1]) + r.Text[1:]
}

type objectNounRequest struct{ Path string }

// objectNoun names the file a validation detail path refers to. Unknown
// prefixes are shown as their readable symbol rather than guessed.
func objectNoun(r objectNounRequest) string {
	switch {
	case strings.HasPrefix(r.Path, "source"):
		return "source file"
	case strings.HasPrefix(r.Path, "target"):
		return "sidecar"
	case strings.HasPrefix(r.Path, "lock"):
		return "writer lock file"
	}
	return readable(readableRequest{r.Path})
}

// ioNouns names every IODetails path; ioOperations phrases every IODetails
// operation, with {noun} replaced by the path's noun. Both cover the schema
// enums exactly (see text_internal_test.go).
var ioNouns = map[string]string{
	"target":    "the sidecar",
	"source":    "the source file",
	"lock":      "the writer lock file",
	"parent":    "the sidecar's directory",
	"temp":      "a temporary file",
	"generator": "the ID generator",
}

const ioCheckAccess = "Check that the file exists and that you can read and write its directory."

var ioOperations = map[string][2]string{
	"open":                          {"Could not open {noun}.", ioCheckAccess},
	"read":                          {"Could not read {noun}.", ioCheckAccess},
	"write":                         {"Could not write {noun}.", "Check free disk space and that you can write the sidecar's directory."},
	"close":                         {"Could not close {noun}.", "Check free disk space, then read the sidecar before retrying."},
	"rename":                        {"Could not replace the sidecar with its updated copy.", "Check that you can write the sidecar's directory."},
	"unlink":                        {"Could not remove {noun}.", "Check that you can write the sidecar's directory."},
	"lock":                          {"Could not take the writer lock.", "Check that you can write the sidecar's directory and that no other program holds the lock."},
	"unlock":                        {"Could not release the writer lock.", "Read the sidecar before retrying."},
	"identity":                      {"Unexpected change to {noun} during the operation.", "Make sure no other program is replacing these files, then retry."},
	"csprng":                        {"Could not generate a new ID.", "Retry, or supply explicit IDs with --id and --message-id."},
	"clock":                         {"Could not generate a new ID because the system clock is unavailable.", "Check the system clock, or supply explicit IDs with --id and --message-id."},
	"randomness_exhausted":          {"Could not generate a new ID.", "Retry, or supply explicit IDs with --id and --message-id."},
	"unsupported_secure_filesystem": {"This filesystem does not support the safe file operations Sidecar needs to write.", "Use a local disk on macOS or Linux."},
	"fsync_parent":                  {"Could not flush {noun} to disk.", "Read the sidecar before retrying."},
	"cleanup":                       {"Could not remove {noun} left by the operation.", "Remove leftover temporary files beside the sidecar when no Sidecar process is running."},
}

type ioTextRequest struct{ Details domain.IODetails }

func ioText(r ioTextRequest) (string, string) {
	d := r.Details
	noun, nounOK := ioNouns[d.Path]
	phrase, operationOK := ioOperations[d.Operation]
	explanation, next := "", ioCheckAccess
	switch {
	case !nounOK || !operationOK:
		explanation = fmt.Sprintf("File operation failed (%s on %s).", readable(readableRequest{d.Operation}), readable(readableRequest{d.Path}))
	case d.Path == "temp" && (d.Operation == "csprng" || d.Operation == "randomness_exhausted"):
		// Randomness here names a temporary file, not an ID.
		explanation, next = "Could not choose a name for a temporary file beside the sidecar.", "Retry the command."
		if d.Operation == "randomness_exhausted" {
			next = "Remove leftover temporary files beside the sidecar while no Sidecar process is running, then retry."
		}
	case d.Operation == "open" && d.Path == "source":
		explanation, next = "Could not open the source file. It is expected beside the sidecar, with the same name minus .comments.", "Check that the source file exists and is readable."
	default:
		explanation, next = strings.ReplaceAll(phrase[0], "{noun}", noun), phrase[1]
	}
	if d.CommitMayHaveSucceeded != nil && *d.CommitMayHaveSucceeded {
		next = "The write may already have been saved. Read the sidecar before retrying."
	}
	return explanation, next
}

type validationTextRequest struct {
	Details domain.ValidationDetails
	File    string
}

type typeTextRequest struct {
	Details domain.ValidationDetails
	File    string
}

// typeText explains a non-regular object. Sources may have several hard links;
// sidecars and lock files may not.
func typeText(r typeTextRequest) (string, string) {
	d := r.Details
	noun := objectNoun(objectNounRequest{Path: d.Path})
	kinds := map[string]string{"symlink": "a symbolic link", "directory": "a directory", "nonregular": "a special file", "multilink": "a file with several hard links"}
	kind, ok := kinds[d.Reason]
	if !ok {
		kind = "unusable (" + readable(readableRequest{d.Reason}) + ")"
	}
	rule := "Sidecar only uses regular, single-link files."
	if d.Path == "source_type" {
		rule = "Sidecar only reads regular files and does not follow links."
	}
	explanation := fmt.Sprintf("The %s is %s; %s", noun, kind, rule)
	switch {
	case d.Path == "lock_type":
		return explanation, fmt.Sprintf("Remove the stray lock entry %q while no Sidecar process is running.", "."+filepath.Base(r.File)+".sidecar.lock")
	case d.Path == "source_type" && d.Reason == "symlink":
		return explanation, "Replace the source link with the file itself; Sidecar finds the source from the sidecar's name."
	case d.Path == "source_type":
		return explanation, "Make sure the source path names a regular file."
	case d.Reason == "symlink":
		return explanation, "Point --file at the real sidecar, or replace the link with the file itself."
	case d.Reason == "directory":
		return explanation, "Remove or rename the directory at this path, or choose another --file."
	case d.Reason == "multilink":
		return explanation, "Remove the extra hard links, or copy the file, so it has a single link."
	}
	return explanation, "Point --file at a regular file."
}

// suggestedTarget returns FILE.comments when that would itself be a valid
// sidecar name, otherwise "".
func suggestedTarget(r validationTextRequest) string {
	base := filepath.Base(r.File)
	if r.File == "" || strings.HasSuffix(r.File, string(filepath.Separator)) || base == "." || base == ".comments" || strings.HasSuffix(base, ".comments") {
		return ""
	}
	return r.File + ".comments"
}

// Human explanations for validation details. Unknown symbols fall back to a
// readable form of the detail so new failures are never silently generic.
func validationText(r validationTextRequest) (string, string) {
	d := r.Details
	if len(d.Diagnostics) > 0 {
		problem := d.Diagnostics[0]
		file := r.File
		if strings.ContainsFunc(file, unicode.IsControl) {
			file = fmt.Sprintf("%q", file)
		}
		return fmt.Sprintf("%s:%d:%d: %s", file, problem.Line, problem.Column, problem.Message), problem.Hint
	}
	noun := objectNoun(objectNounRequest{Path: d.Path})
	switch {
	case d.Path == "source_coordinate":
		return "The location is not inside the source file: a line is past the end, a column is past the end of its line, or a column splits a multibyte character. Columns count UTF-8 bytes.",
			"Run sidecar source inspect --file PATH to see the source's line count, then choose a location inside it."
	case d.Path == "target_name":
		next := "Pass the sidecar path with --file, for example --file chapter.md.comments for chapter.md."
		if suggestion := suggestedTarget(r); suggestion != "" {
			next = fmt.Sprintf("Pass the sidecar path with --file, for example --file %q.", suggestion)
		}
		return "Sidecar files are named after their source plus .comments, for example chapter.md.comments for chapter.md.", next
	case strings.HasSuffix(d.Path, "_type"):
		return typeText(typeTextRequest(r))
	case strings.HasSuffix(d.Path, "_encoding"):
		problems := map[string]string{"bom": "starts with a byte-order mark", "nul": "contains a NUL byte", "utf8": "is not valid UTF-8"}
		problem, ok := problems[d.Reason]
		if !ok {
			problem = "has an encoding problem (" + readable(readableRequest{d.Reason}) + ")"
		}
		return fmt.Sprintf("The %s %s; Sidecar reads only plain UTF-8 text.", noun, problem),
			fmt.Sprintf("Save the %s as UTF-8 without a byte-order mark.", noun)
	case d.Path == "target_endings":
		return "Sidecar files must use LF line endings.",
			"Run sidecar format --file PATH to convert consistent CRLF line endings; fix bare carriage returns by hand."
	case d.Path == "source_endings":
		return "The source file mixes line endings or uses bare carriage returns.",
			"Save the source file with consistent LF or CRLF line endings."
	case d.Path == "source_size":
		return fmt.Sprintf("The source file is larger than the %d MiB limit.", domain.MaxSourceBytes>>20), "Split the source into smaller files."
	case d.Path == "target_size":
		return fmt.Sprintf("The sidecar is or would become larger than the %d MiB limit.", domain.MaxTargetBytes>>20),
			"If the file is already too large, trim or split it using version control; otherwise delete threads you no longer need."
	case d.Path == "tracking":
		explanations := map[string]string{"invalid": "Stored tracking data in the sidecar is malformed or inconsistent.", "too_large": "Stored tracking data exceeds Sidecar's tracking size limits."}
		explanation, ok := explanations[d.Reason]
		if !ok {
			explanation = "Stored tracking data in the sidecar is unusable (" + readable(readableRequest{d.Reason}) + ")."
		}
		return explanation, "Restore the sidecar from version control or a backup; Sidecar cannot read it until the tracking line is fixed."
	case d.Path == "target_content":
		return "The sidecar is not in a valid Sidecar format.",
			"Run sidecar validate --file PATH to find the problem; resolve any merge conflicts first."
	}
	return fmt.Sprintf("The %s failed validation (%s: %s).", noun, readable(readableRequest{d.Path}), readable(readableRequest{d.Reason})),
		"Run sidecar validate --file PATH for details."
}

type conflictTextRequest struct{ Details domain.ConflictDetails }

func conflictText(r conflictTextRequest) (string, string) {
	switch r.Details.Kind {
	case "revision":
		return "The sidecar changed since its revision was read.",
			"Read the sidecar again and retry with the current revision."
	case "source_revision":
		return "The source file changed since its revision was captured.",
			"Run sidecar source inspect --file PATH for the current revision, recompute locations, and retry."
	case "lock_timeout":
		return "Another Sidecar process is holding the writer lock.", "Wait for it to finish, then retry."
	case "identity_changed":
		return "The sidecar was replaced by another program during the update.", "Read the sidecar again before retrying."
	}
	return fmt.Sprintf("The operation conflicts with the current state (%s).", readable(readableRequest{r.Details.Kind})),
		"Read the sidecar again before retrying."
}

type preconditionTextRequest struct {
	Details operations.PreconditionDetails
}

func preconditionText(r preconditionTextRequest) (string, string) {
	d := r.Details
	item := ""
	if d.Index != nil {
		item = fmt.Sprintf(" (item %d, index %d)", *d.Index+1, *d.Index)
	}
	explanations := map[string][2]string{
		"retry_mismatch":          {"A thread or message with this ID already exists with different content.", "Use a new ID, or repeat the original request exactly."},
		"id_collision":            {"This ID is already used by a different thread or message.", "Use a new ID."},
		"if_last_author_kind":     {"The thread's last message is not by the expected author kind.", "Read the thread to see who replied last."},
		"limit_threads":           {fmt.Sprintf("The sidecar already has the maximum of %d threads.", domain.MaxThreads), "Delete threads you no longer need."},
		"limit_messages_thread":   {fmt.Sprintf("The thread already has the maximum of %d messages.", domain.MaxMessagesThread), "Start a new thread."},
		"limit_messages_document": {fmt.Sprintf("The sidecar already has the maximum of %d messages.", domain.MaxMessagesDocument), "Delete messages or threads you no longer need."},
		"limit_selection":         {fmt.Sprintf("The selected text is larger than the %d-byte selection limit.", domain.MaxAnchorBytes), "Select a shorter range."},
		"limit_tracking":          {fmt.Sprintf("Retained tracking text for all threads would exceed the %d MiB limit.", domain.MaxTrackingBytes>>20), "Move some threads to file or lost, or select shorter ranges."},
		"duplicate_thread":        {"The same thread appears more than once in the batch.", "Send each thread once."},
		"not_tracked":             {"The thread has no retained tracking to mark as deleted.", "File and lost threads have no tracking to report deleted."},
	}
	text, ok := explanations[d.Reason]
	if !ok {
		text = [2]string{fmt.Sprintf("The %s precondition was not met (%s).", readable(readableRequest{d.Operation}), readable(readableRequest{d.Reason})), "Read the current thread and check the request before retrying."}
	}
	return strings.TrimSuffix(text[0], ".") + item + ".", text[1]
}
