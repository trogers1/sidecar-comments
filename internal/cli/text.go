package cli

import (
	"encoding/json"
	"fmt"
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
			writeThread(threadTextRequest{&b, t})
		}
		writeWarnings(warningsTextRequest{&b, v.Warnings})
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
		writeThread(threadTextRequest{r.Writer, *r.Result.Thread})
	}
	if r.Result.Message != nil {
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
	Writer *strings.Builder
	Thread domain.Thread
}

func writeThread(r threadTextRequest) {
	t := r.Thread
	l := t.Location
	location := l.Kind
	switch l.Kind {
	case domain.LocationLine:
		location = fmt.Sprintf("line %d", l.Line)
	case domain.LocationRange:
		location = fmt.Sprintf("range %d:%d–%d:%d", l.StartLine, l.StartColumn, l.EndLine, l.EndColumn)
	}
	fmt.Fprintf(r.Writer, "\nThread %s — %s\n", t.ID, location)
	if l.Anchor != nil {
		fmt.Fprintf(r.Writer, "  Anchor: %s\n", *l.Anchor)
	}
	for _, m := range t.Messages {
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
			explanation = "The input failed validation: " + readable(readableRequest{e.Message}) + "."
			next = "Check the sidecar syntax and source locations, then run validate again."
		case domain.CodeConflict:
			explanation = "The operation conflicts with the current state: " + fmt.Sprintf("%q", readable(readableRequest{e.Message})) + "."
			next = "Read the latest sidecar and check IDs, revision, or writer lock before retrying."
		case domain.CodePrecondition:
			explanation = "The requested operation's precondition was not met: " + fmt.Sprintf("%q", readable(readableRequest{e.Message})) + "."
			next = "Read the current thread and check the operation's conditions before retrying."
		case domain.CodeIO:
			explanation = "The file operation failed: " + fmt.Sprintf("%q", e.Message) + "."
			next = "Check paths, permissions, and writer locks. A write may already have committed; inspect the sidecar before retrying."
		case domain.CodeUnsupported:
			explanation = "This platform, protocol, or format is not supported."
			next = "Check sidecar version and use a supported platform and input version."
		case domain.CodeRepair:
			explanation = "The sidecar has missing message IDs and needs repair."
			next = "Run sidecar fix --file PATH --add-missing-message-ids."
		default:
			explanation = readable(readableRequest{e.Message}) + "."
		}
	}
	hint := "sidecar --help"
	if _, ok := commandDefinitions[c.command]; ok {
		hint = "sidecar " + strings.ReplaceAll(c.command, "-", " ") + " --help"
	}
	target := ""
	if c.file != "" {
		target = fmt.Sprintf("Sidecar: %q\n", c.file)
	}
	return fmt.Sprintf("error %s: %s\n%s%s\nFor usage, run: %s\n", e.Code, explanation, target, next, hint)
}
