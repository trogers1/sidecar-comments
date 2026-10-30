package cli

import (
	"fmt"
	"sort"
	"strings"
)

type flagDoc struct{ value, description, example string }

var flagDocs = map[string]flagDoc{
	"file":                    {"PATH", "Sidecar file (the source path plus .comments).", "proposal.md.comments"},
	"source":                  {"PATH", "Validate locations against this UTF-8 source file.", "proposal.md"},
	"id":                      {"ID", "Thread or message ID; optional on creation (generated if omitted).", "01900000-0000-7000-8000-000000000001"},
	"message-id":              {"ID", "Initial message ID; supply together with --id.", "01900000-0000-7000-8000-000000000002"},
	"thread":                  {"ID", "Parent thread ID.", "01900000-0000-7000-8000-000000000001"},
	"location":                {"LOCATION", "Location: file, lost, line:N, or range:L:C-L:C (byte columns).", "line:1"},
	"author-kind":             {"KIND", "Author kind: human, agent, or external.", "human"},
	"author-name":             {"NAME", "Author name (required for external authors).", "Taylor"},
	"external-author-id":      {"ID", "External author ID; only for external authors.", "reviewer-42"},
	"body-file":               {"PATH", "Read the message body from a UTF-8 file; - reads stdin.", "note.txt"},
	"gitlab-json":             {"PATH", "Read GitLab metadata JSON; - reads stdin.", "gitlab.json"},
	"operations-file":         {"PATH", "Read a protocol-versioned operations JSON document; - reads stdin.", "operations.json"},
	"if-revision":             {"REVISION", "Require this storage revision (or missing); place before ordinary flags.", "missing"},
	"lock-timeout":            {"DURATION", "Writer lock timeout: 1ms–60s (default 5s); place before ordinary flags.", "5s"},
	"if-last-author-kind":     {"KIND", "Only append if the last message has this author kind.", "human"},
	"check":                   {"", "Check without writing; exit with the validation status when changes are needed.", ""},
	"add-missing-message-ids": {"", "Assign missing message IDs (required for fix).", ""},
}

type helpRequest struct{ Command string }

func helpText(r helpRequest) string {
	var b strings.Builder
	root := r.Command == ""
	group := r.Command == "thread" || r.Command == "message"
	if root {
		b.WriteString("🛵 Sidecar — Comments ride alongside your text\n\nUsage: sidecar [--output text|json|id|none] COMMAND [FLAGS]\n\nCommands:\n")
	} else if group {
		fmt.Fprintf(&b, "Usage: sidecar %s COMMAND [FLAGS]\n\nCommands:\n", r.Command)
	} else {
		fmt.Fprintf(&b, "%s\n\nUsage: sidecar [--output text|json|id|none] %s [FLAGS]\n\nFlags:\n", commandDefinitions[r.Command].description, strings.ReplaceAll(r.Command, "-", " "))
	}
	if root || group {
		names := make([]string, 0, len(commandDefinitions))
		for name := range commandDefinitions {
			if root || strings.HasPrefix(name, r.Command+"-") {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(&b, "  %-20s %s\n", strings.ReplaceAll(name, "-", " "), commandDefinitions[name].description)
		}
	} else {
		d := commandDefinitions[r.Command]
		required := " " + d.required + " "
		for _, name := range strings.Fields(commandFlags(commandFlagsRequest{Definition: d})) {
			doc, ok := flagDocs[name]
			if !ok {
				panic("missing flag documentation: " + name)
			}
			suffix := ""
			if name == "file" || strings.Contains(required, " "+name+" ") || r.Command == "fix" && name == "add-missing-message-ids" {
				suffix = " (required)"
			}
			fmt.Fprintf(&b, "  --%-24s %s%s\n", strings.TrimSpace(name+" "+doc.value), doc.description, suffix)
		}
		fmt.Fprintf(&b, "\nExample:\n  sidecar %s", strings.ReplaceAll(r.Command, "-", " "))
		for _, name := range strings.Fields(d.allowed) {
			if name == "file" || strings.Contains(required, " "+name+" ") || r.Command == "fix" && name == "add-missing-message-ids" {
				fmt.Fprintf(&b, " --%s", name)
				if flagDocs[name].value != "" {
					fmt.Fprintf(&b, " %s", flagDocs[name].example)
				}
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("\n  -h, --help  Show help without reading files or stdin, or writing files.\n\nOutput: text is human-readable; json is the versioned machine envelope.\nUse --output before the command. id is available for thread create, message add,\nand fix; none is available for mutations. Help is always plain text, even with\n--output json. A flag value named --help or -h is a value, not a help request.\n")
	if root {
		b.WriteString("\nStart here: sidecar thread create --help\nVersion: sidecar --version (or sidecar version)\n")
	} else {
		b.WriteString("\nSee also: sidecar --help\n")
	}
	return b.String()
}

type commandFlagsRequest struct{ Definition commandDefinition }

func commandFlags(r commandFlagsRequest) string {
	flags := r.Definition.allowed
	if r.Definition.mutation {
		flags += " if-revision lock-timeout"
	}
	return flags
}
