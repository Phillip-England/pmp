package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

type compileRange struct {
	Start int
	End   int
}

type compileTarget struct {
	Range               *compileRange
	FromMark            bool
	OutputFile          string
	IncludedSkills      []string
	IncludeInstructions bool
	UpdateMark          bool
	ToStdout            bool
}

func parseCompileArgs(args []string) (compileTarget, error) {
	target := compileTarget{
		IncludeInstructions: true,
		UpdateMark:          true,
	}

	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--from-mark":
			if target.Range != nil || target.FromMark {
				return compileTarget{}, errors.New("choose only one compile source: default all, `--from-mark`, or `--range <start> <end>`")
			}
			target.FromMark = true
		case arg == "--range":
			if target.Range != nil || target.FromMark {
				return compileTarget{}, errors.New("choose only one compile source: default all, `--from-mark`, or `--range <start> <end>`")
			}
			if i+2 >= len(args) {
				return compileTarget{}, errors.New("usage: `pmp compile --range <start> <end>`")
			}
			start, err := strconv.Atoi(strings.TrimSpace(args[i+1]))
			if err != nil {
				return compileTarget{}, fmt.Errorf("invalid start index %q", args[i+1])
			}
			end, err := strconv.Atoi(strings.TrimSpace(args[i+2]))
			if err != nil {
				return compileTarget{}, fmt.Errorf("invalid end index %q", args[i+2])
			}
			target.Range = &compileRange{Start: start, End: end}
			i += 2
		case arg == "--output":
			if i+1 >= len(args) {
				return compileTarget{}, errors.New("usage: `pmp compile --output <file>`")
			}
			target.OutputFile = strings.TrimSpace(args[i+1])
			if target.OutputFile == "" {
				return compileTarget{}, errors.New("output file cannot be empty")
			}
			i++
		case arg == "--stdout":
			target.ToStdout = true
		case strings.HasPrefix(arg, "--update-mark="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--update-mark="))
			parsed, err := parseBoolFlag(value)
			if err != nil {
				return compileTarget{}, fmt.Errorf("invalid --update-mark value %q", value)
			}
			target.UpdateMark = parsed
		case arg == "--update-mark":
			target.UpdateMark = true
		case strings.HasPrefix(arg, "--include-instructions="):
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--include-instructions="))
			parsed, err := parseBoolFlag(value)
			if err != nil {
				return compileTarget{}, fmt.Errorf("invalid --include-instructions value %q", value)
			}
			target.IncludeInstructions = parsed
		case arg == "--include-instructions":
			target.IncludeInstructions = true
		case arg == "--skill":
			if i+1 >= len(args) {
				return compileTarget{}, errors.New("usage: `pmp compile --skill <name>`")
			}
			name := strings.TrimSpace(args[i+1])
			if name == "" {
				return compileTarget{}, errors.New("skill name cannot be empty")
			}
			target.IncludedSkills = append(target.IncludedSkills, name)
			i++
		case strings.HasPrefix(arg, "--skills="):
			names, err := parseSkillsFlagValue(strings.TrimPrefix(arg, "--skills="))
			if err != nil {
				return compileTarget{}, err
			}
			target.IncludedSkills = append(target.IncludedSkills, names...)
		case arg == "--skills":
			if i+1 >= len(args) {
				return compileTarget{}, errors.New("usage: `pmp compile --skills <comma-separated names>`")
			}
			names, err := parseSkillsFlagValue(args[i+1])
			if err != nil {
				return compileTarget{}, err
			}
			target.IncludedSkills = append(target.IncludedSkills, names...)
			i++
		default:
			return compileTarget{}, fmt.Errorf("unknown compile argument %q", arg)
		}
	}

	if target.OutputFile != "" && target.ToStdout {
		return compileTarget{}, errors.New("choose only one output mode: `--output <file>` or `--stdout`")
	}
	target.IncludedSkills = dedupeStrings(target.IncludedSkills)
	return target, nil
}

func parseBoolFlag(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, errors.New("invalid boolean")
	}
}

func parseSkillsFlagValue(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "{")
	raw = strings.TrimSuffix(raw, "}")
	if raw == "" {
		return nil, errors.New("skills list cannot be empty")
	}

	parts := strings.Split(raw, ",")
	names := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		name = strings.Trim(name, `"'`)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return nil, errors.New("skills list cannot be empty")
	}
	return names, nil
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, strings.TrimSpace(value))
	}
	return out
}

func runCompileCommand(args []string) error {
	if len(args) > 0 && strings.EqualFold(strings.TrimSpace(args[0]), "history") {
		target, err := parseCompileArgs(args[1:])
		if err != nil {
			return err
		}
		return runCompileHistory(target)
	}
	target, err := parseCompileArgs(args)
	if err != nil {
		return err
	}
	return runCompile(target)
}

func runCompile(target compileTarget) error {
	prompts, err := loadPrompts()
	if err != nil {
		return err
	}
	projectInstructions, err := loadProjectInstructions()
	if err != nil {
		return err
	}
	memories, err := loadMemories()
	if err != nil {
		return err
	}
	skills, err := loadSkills()
	if err != nil {
		return err
	}

	selected, err := resolveCompileIndexesCLI(target, len(prompts))
	if err != nil {
		return err
	}

	compiled, err := compilePromptIndexes(prompts, selected, skills, target.IncludedSkills)
	if err != nil {
		return err
	}
	compiled = assembleCompiledDocument(projectInstructions, memories, compiled, target.IncludeInstructions)

	if target.UpdateMark && len(selected) > 0 {
		if err := markCompiledPrompt(selected); err != nil {
			return err
		}
	}

	switch {
	case target.OutputFile != "":
		return writeCompiledFile(target.OutputFile, compiled)
	case target.ToStdout:
		_, err := os.Stdout.WriteString(compiled)
		return err
	default:
		return copyCompiledToClipboard(compiled)
	}
}

func runCompileHistory(target compileTarget) error {
	if target.FromMark {
		return errors.New("history compile does not support `--from-mark`")
	}
	if len(target.IncludedSkills) > 0 {
		return errors.New("history compile does not support skill selection")
	}

	history, err := loadHistory()
	if err != nil {
		return err
	}
	projectInstructions, err := loadProjectInstructions()
	if err != nil {
		return err
	}
	memories, err := loadMemories()
	if err != nil {
		return err
	}

	selected, err := resolveCompileIndexesCLI(target, len(history))
	if err != nil {
		return err
	}

	compiled, err := compileHistoryIndexes(history, selected)
	if err != nil {
		return err
	}
	compiled = assembleHistoryCompiledDocument(projectInstructions, memories, compiled, target.IncludeInstructions)

	switch {
	case target.OutputFile != "":
		return writeCompiledFile(target.OutputFile, compiled)
	case target.ToStdout:
		_, err := os.Stdout.WriteString(compiled)
		return err
	default:
		return copyCompiledToClipboard(compiled)
	}
}

func resolveCompileIndexesCLI(target compileTarget, promptCount int) ([]int, error) {
	switch {
	case target.FromMark:
		marks, err := loadMarks()
		if err != nil {
			return nil, err
		}
		return indexesFromMarkExclusive(promptCount, marks)
	case target.Range != nil:
		return compileIndexesForRange(promptCount, *target.Range)
	default:
		indexes := make([]int, 0, promptCount)
		for i := 0; i < promptCount; i++ {
			indexes = append(indexes, i)
		}
		return indexes, nil
	}
}

func compilePrompts(prompts []Prompt, rng *compileRange, skills []Skill, includedSkills []string) (string, error) {
	indexes, err := resolveCompileIndexesCLI(compileTarget{Range: rng}, len(prompts))
	if err != nil {
		return "", err
	}
	return compilePromptIndexes(prompts, indexes, skills, includedSkills)
}

func compileHistoryIndexes(entries []Prompt, indexes []int) (string, error) {
	sorted := append([]int(nil), indexes...)
	sort.Ints(sorted)

	var historyBody strings.Builder
	for _, i := range sorted {
		if i < 0 || i >= len(entries) {
			return "", fmt.Errorf("compile index %d is out of bounds; highest history index is %d", i, len(entries)-1)
		}
		entry := entries[i]
		if historyBody.Len() > 0 {
			historyBody.WriteString("\n\n")
		}
		historyBody.WriteString("<!-- history ")
		historyBody.WriteString(strconv.Itoa(i))
		historyBody.WriteString(" | ")
		historyBody.WriteString(entry.Timestamp.Format(time.RFC3339))
		historyBody.WriteString(" -->\n")
		historyBody.WriteString(strings.TrimSpace(entry.Markdown))
	}
	return renderHistoryCompileDocument("", nil, historyBody.String()), nil
}

func compilePromptIndexes(prompts []Prompt, indexes []int, skills []Skill, includedSkills []string) (string, error) {
	sorted := append([]int(nil), indexes...)
	sort.Ints(sorted)

	var promptBody strings.Builder
	for _, i := range sorted {
		if i < 0 || i >= len(prompts) {
			return "", fmt.Errorf("compile index %d is out of bounds; highest prompt index is %d", i, len(prompts)-1)
		}
		prompt := prompts[i]
		if promptBody.Len() > 0 {
			promptBody.WriteString("\n\n")
		}
		promptBody.WriteString("<!-- prompt ")
		promptBody.WriteString(strconv.Itoa(i))
		promptBody.WriteString(" | ")
		promptBody.WriteString(prompt.Timestamp.Format(time.RFC3339))
		promptBody.WriteString(" -->\n")
		promptBody.WriteString(strings.TrimSpace(prompt.Markdown))
	}
	return renderCompileDocument("", []Memory{}, renderSelectedSkills(skills, includedSkills), promptBody.String()), nil
}

func renderSelectedSkills(skills []Skill, includedSkills []string) string {
	included := skillNamesSet(includedSkills)
	var blocks []string
	for _, skill := range skills {
		if !included[normalizeSkillName(skill.Name)] {
			continue
		}
		body := strings.TrimSpace(skill.Body)
		if body == "" {
			continue
		}
		blocks = append(blocks, body)
	}
	return strings.Join(blocks, "\n\n")
}

func assembleCompiledDocument(instructions string, memories []Memory, compiled string, includeInstructions bool) string {
	skills := extractCompileSection("Skills", compiled)
	prompts := extractCompileSection("Prompts", compiled)
	if skills == "" && prompts == "" {
		prompts = compiled
	}
	if !includeInstructions {
		instructions = ""
	}
	return renderCompileDocument(instructions, memories, skills, prompts)
}

func assembleHistoryCompiledDocument(instructions string, memories []Memory, compiled string, includeInstructions bool) string {
	history := extractCompileSection("History", compiled)
	if history == "" {
		history = compiled
	}
	if !includeInstructions {
		instructions = ""
	}
	return renderHistoryCompileDocument(instructions, memories, history)
}

func prefixCompiledWithInstructions(instructions string, memories []Memory, compiled string) string {
	return assembleCompiledDocument(instructions, memories, compiled, true)
}

func renderCompileDocument(instructions string, memories []Memory, skills string, prompts string) string {
	var b strings.Builder
	if strings.TrimSpace(instructions) != "" {
		writeCompileSection(&b, "Instructions", "Read this section first. It explains how to use the compiled material and how to write history notes back into the project.", instructions, "_No instructions provided._")
		b.WriteString("\n\n")
	}
	b.WriteString("<!-- MEMORY SECTION -->\n")
	b.WriteString("# Memory Section\n")
	b.WriteString("This section contains project-specific context that should be applied throughout the work.\n\n")
	if len(memories) == 0 {
		b.WriteString("_No memories recorded._")
	} else {
		for _, m := range memories {
			b.WriteString(strings.TrimSpace(m.Body))
			b.WriteString("\n\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n\n")
	writeCompileSection(&b, "Skills", "These are the optional selected skills included for this compilation.", skills, "_No skills selected._")
	b.WriteString("\n\n")
	writeCompileSection(&b, "Prompts", "These are the selected prompts in chronological order.", prompts, "_No prompts selected._")
	b.WriteByte('\n')
	return b.String()
}

func renderHistoryCompileDocument(instructions string, memories []Memory, history string) string {
	var b strings.Builder
	if strings.TrimSpace(instructions) != "" {
		writeCompileSection(&b, "Instructions", "Read this section first. It explains how to use the compiled material and how to write history notes back into the project.", instructions, "_No instructions provided._")
		b.WriteString("\n\n")
	}
	b.WriteString("<!-- MEMORY SECTION -->\n")
	b.WriteString("# Memory Section\n")
	b.WriteString("This section contains project-specific context that should be applied throughout the work.\n\n")
	if len(memories) == 0 {
		b.WriteString("_No memories recorded._")
	} else {
		for _, m := range memories {
			b.WriteString(strings.TrimSpace(m.Body))
			b.WriteString("\n\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n\n")
	b.WriteString("<!-- HISTORY SECTION -->\n")
	b.WriteString("# History Section\n")
	b.WriteString("This section contains project history recorded by the assistant who previously made changes. Treat it as implementation history from the assistant's perspective.\n\n")
	history = strings.TrimSpace(history)
	if history == "" {
		b.WriteString("_No history selected._")
	} else {
		b.WriteString(history)
	}
	b.WriteByte('\n')
	return b.String()
}

func writeCompileSection(b *strings.Builder, title string, description string, body string, emptyMessage string) {
	b.WriteString("<!-- ")
	b.WriteString(strings.ToUpper(title))
	b.WriteString(" SECTION -->\n")
	b.WriteString("# ")
	b.WriteString(title)
	b.WriteString(" Section\n")
	b.WriteString(description)
	b.WriteString("\n\n")
	body = strings.TrimSpace(body)
	if body == "" {
		b.WriteString(emptyMessage)
		return
	}
	b.WriteString(body)
}

func extractCompileSection(title string, compiled string) string {
	startMarker := "<!-- " + strings.ToUpper(title) + " SECTION -->"
	nextMarkers := []string{
		"<!-- INSTRUCTIONS SECTION -->",
		"<!-- MEMORY SECTION -->",
		"<!-- SKILLS SECTION -->",
		"<!-- PROMPTS SECTION -->",
		"<!-- HISTORY SECTION -->",
	}
	start := strings.Index(compiled, startMarker)
	if start == -1 {
		return ""
	}
	section := compiled[start:]
	next := len(section)
	for _, marker := range nextMarkers {
		if marker == startMarker {
			continue
		}
		if idx := strings.Index(section[len(startMarker):], marker); idx != -1 {
			candidate := len(startMarker) + idx
			if candidate < next {
				next = candidate
			}
		}
	}
	section = strings.TrimSpace(section[:next])
	lines := strings.Split(section, "\n")
	if len(lines) <= 3 {
		return ""
	}
	return strings.TrimSpace(strings.Join(lines[3:], "\n"))
}

func writeCompiledFile(path string, compiled string) error {
	if err := os.WriteFile(path, []byte(compiled), 0o644); err != nil {
		return err
	}
	fmt.Println("compiled document written to " + filepath.Clean(path))
	return nil
}

func copyCompiledToClipboard(compiled string) error {
	command, args, err := clipboardCommand()
	if err != nil {
		return err
	}
	cmd := exec.Command(command, args...)
	cmd.Stdin = strings.NewReader(compiled)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("copy to clipboard with %s failed: %w (%s)", command, err, strings.TrimSpace(string(output)))
	}
	fmt.Println("compiled document copied to clipboard")
	return nil
}

func clipboardCommand() (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		return "pbcopy", nil, nil
	case "linux":
		if _, err := exec.LookPath("wl-copy"); err == nil {
			return "wl-copy", nil, nil
		}
		if _, err := exec.LookPath("xclip"); err == nil {
			return "xclip", []string{"-selection", "clipboard"}, nil
		}
		return "", nil, errors.New("no clipboard command found; install `wl-copy` or `xclip`, or pass `--stdout` or `--output <file>`")
	case "windows":
		return "clip", nil, nil
	default:
		return "", nil, fmt.Errorf("clipboard copy is not supported on %s; pass `--stdout` or `--output <file>`", runtime.GOOS)
	}
}
