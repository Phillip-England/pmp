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
	Range      *compileRange
	OutputFile string
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

	prefix, err := loadPrefix()
	if err != nil {
		return err
	}

	compiled, err := compilePrompts(prompts, target.Range, prefix)
	if err != nil {
		return err
	}

	if target.OutputFile != "" {
		return writeCompiledFile(target.OutputFile, compiled)
	}
	return copyCompiledToClipboard(compiled)
}

func compilePrompts(prompts []Prompt, rng *compileRange, prefix string) (string, error) {
	if len(prompts) == 0 {
		return strings.TrimSpace(prefix), nil
	}

	start := 0
	end := len(prompts) - 1
	if rng != nil {
		start = rng.Start
		end = rng.End
	}

	if start < 0 || end < 0 {
		return "", errors.New("compile range indexes must be non-negative")
	}
	if start > end {
		return "", errors.New("compile start index must be less than or equal to end index")
	}
	if end >= len(prompts) {
		return "", fmt.Errorf("compile range out of bounds; highest prompt index is %d", len(prompts)-1)
	}

	indexes := make([]int, 0, end-start+1)
	for i := start; i <= end; i++ {
		indexes = append(indexes, i)
	}
	return compilePromptIndexes(prompts, indexes, prefix)
}

func compilePromptIndexes(prompts []Prompt, indexes []int, prefix string) (string, error) {
	sorted := append([]int(nil), indexes...)
	sort.Ints(sorted)

	var b strings.Builder
	trimmedPrefix := strings.TrimSpace(prefix)
	if trimmedPrefix != "" {
		b.WriteString(trimmedPrefix)
	}
	for _, i := range sorted {
		if i < 0 || i >= len(prompts) {
			return "", fmt.Errorf("compile index %d is out of bounds; highest prompt index is %d", i, len(prompts)-1)
		}
		prompt := prompts[i]
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("<!-- prompt ")
		b.WriteString(strconv.Itoa(i))
		b.WriteString(" | ")
		b.WriteString(prompt.Timestamp.Format(time.RFC3339))
		b.WriteString(" -->\n")
		b.WriteString(strings.TrimSpace(prompt.Markdown))
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func loadPrefix() (string, error) {
	if err := ensureProject(); err != nil {
		return "", err
	}

	path, err := prefixPath()
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func savePrefix(prefix string) error {
	if err := ensureProject(); err != nil {
		return err
	}

	path, err := prefixPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(prefix), 0o644)
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
