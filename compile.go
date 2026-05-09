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
	Range          *compileRange
	OutputFile     string
	IncludedSkills []string
}

func parseCompileArgs(args []string) (compileTarget, error) {
	switch len(args) {
	case 0:
		return compileTarget{}, nil
	case 1:
		return compileTarget{OutputFile: args[0]}, nil
	case 2:
		start, startErr := strconv.Atoi(args[0])
		end, endErr := strconv.Atoi(args[1])
		if startErr == nil && endErr == nil {
			return compileTarget{Range: &compileRange{Start: start, End: end}}, nil
		}
		return compileTarget{}, errors.New("usage: `pmp compile`, `pmp compile <file>`, `pmp compile <start> <end>`, or `pmp compile <start> <end> <file>`")
	case 3:
		start, err := strconv.Atoi(args[0])
		if err != nil {
			return compileTarget{}, fmt.Errorf("invalid start index %q", args[0])
		}
		end, err := strconv.Atoi(args[1])
		if err != nil {
			return compileTarget{}, fmt.Errorf("invalid end index %q", args[1])
		}
		return compileTarget{
			Range:      &compileRange{Start: start, End: end},
			OutputFile: args[2],
		}, nil
	default:
		return compileTarget{}, errors.New("usage: `pmp compile`, `pmp compile <file>`, `pmp compile <start> <end>`, or `pmp compile <start> <end> <file>`")
	}
}

func runCompileCommand(args []string) error {
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
	skills, err := loadSkills()
	if err != nil {
		return err
	}

	compiled, err := compilePrompts(prompts, target.Range, skills, target.IncludedSkills)
	if err != nil {
		return err
	}
	compiled = prefixCompiledWithInstructions(projectInstructions, compiled)

	if target.OutputFile != "" {
		return writeCompiledFile(target.OutputFile, compiled)
	}
	return copyCompiledToClipboard(compiled)
}

func compilePrompts(prompts []Prompt, rng *compileRange, skills []Skill, includedSkills []string) (string, error) {
	start := 0
	end := len(prompts) - 1
	if rng != nil && len(prompts) > 0 {
		start = rng.Start
		end = rng.End
	}

	if rng != nil && (start < 0 || end < 0) {
		return "", errors.New("compile range indexes must be non-negative")
	}
	if rng != nil && start > end {
		return "", errors.New("compile start index must be less than or equal to end index")
	}
	if rng != nil && end >= len(prompts) {
		return "", fmt.Errorf("compile range out of bounds; highest prompt index is %d", len(prompts)-1)
	}

	indexes := []int{}
	if len(prompts) > 0 {
		indexes = make([]int, 0, end-start+1)
		for i := start; i <= end; i++ {
			indexes = append(indexes, i)
		}
	}
	return compilePromptIndexes(prompts, indexes, skills, includedSkills)
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
	return renderCompileDocument("", renderSelectedSkills(skills, includedSkills), promptBody.String()), nil
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

func prefixCompiledWithInstructions(instructions string, compiled string) string {
	skills := extractCompileSection("Skills", compiled)
	prompts := extractCompileSection("Prompts", compiled)
	if skills == "" && prompts == "" {
		prompts = compiled
	}
	return renderCompileDocument(instructions, skills, prompts)
}

func renderCompileDocument(instructions string, skills string, prompts string) string {
	var b strings.Builder
	writeCompileSection(&b, "Instructions", "Read this section first. It explains how to use the compiled material and how to write response notes back into the project.", instructions, "_No instructions provided._")
	b.WriteString("\n\n")
	writeCompileSection(&b, "Skills", "These are the optional selected skills included for this compilation.", skills, "_No skills selected._")
	b.WriteString("\n\n")
	writeCompileSection(&b, "Prompts", "These are the selected prompts in chronological order.", prompts, "_No prompts selected._")
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
		"<!-- SKILLS SECTION -->",
		"<!-- PROMPTS SECTION -->",
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
	fmt.Println("compiled prompts written to " + filepath.Clean(path))
	return nil
}

func copyCompiledToClipboard(compiled string) error {
	command, args, err := clipboardCommand()
	if err != nil {
		return err
	}
	cmd := exec.Command(command, args...)
	cmd.Stdin = strings.NewReader(compiled)
	if err := cmd.Run(); err != nil {
		return err
	}
	fmt.Println("compiled prompts copied to clipboard")
	return nil
}

func clipboardCommand() (string, []string, error) {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("pbcopy"); err == nil {
			return "pbcopy", nil, nil
		}
	case "linux":
		if _, err := exec.LookPath("xclip"); err == nil {
			return "xclip", []string{"-selection", "clipboard"}, nil
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			return "xsel", []string{"--clipboard", "--input"}, nil
		}
	case "windows":
		if _, err := exec.LookPath("clip"); err == nil {
			return "clip", nil, nil
		}
	}
	return "", nil, errors.New("no clipboard command available; provide a target file to `pmp compile` instead")
}
