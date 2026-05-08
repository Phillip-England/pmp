package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var headingPattern = regexp.MustCompile(`(?m)^#\s+(.+?)\s*$`)

type Prompt struct {
	Title     string
	Timestamp time.Time
	Markdown  string
	Path      string
}

func parseDraft(markdown string) (Prompt, error) {
	match := headingPattern.FindStringSubmatch(markdown)
	if len(match) < 2 {
		return Prompt{}, errors.New("prompt must include a markdown title line beginning with `# `")
	}

	title := strings.TrimSpace(match[1])
	if title == "" {
		return Prompt{}, errors.New("prompt title cannot be empty")
	}

	body := extractBody(markdown)
	if strings.TrimSpace(body) == "" {
		return Prompt{}, errors.New("prompt must include body text below the title")
	}

	return Prompt{
		Title:     title,
		Timestamp: time.Now().UTC(),
		Markdown:  strings.TrimSpace(markdown) + "\n",
	}, nil
}

func parsePromptFields(title string, body string) (Prompt, error) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)
	if title == "" {
		return Prompt{}, errors.New("prompt title cannot be empty")
	}
	if body == "" {
		return Prompt{}, errors.New("prompt body cannot be empty")
	}
	return Prompt{
		Title:     title,
		Timestamp: time.Now().UTC(),
		Markdown:  fmt.Sprintf("# %s\n\n%s\n", title, body),
	}, nil
}

func extractBody(markdown string) string {
	lines := strings.Split(markdown, "\n")
	bodyLines := make([]string, 0, len(lines))
	titleSeen := false
	for _, line := range lines {
		if !titleSeen && strings.HasPrefix(strings.TrimSpace(line), "#") {
			titleSeen = true
			continue
		}
		if titleSeen {
			bodyLines = append(bodyLines, line)
		}
	}
	return strings.Join(bodyLines, "\n")
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "prompt"
	}
	return slug
}

func formatPromptFile(prompt Prompt) string {
	return fmt.Sprintf("---\ntitle: %s\ntimestamp: %s\n---\n\n%s",
		escapeYAML(prompt.Title),
		prompt.Timestamp.Format(time.RFC3339),
		prompt.Markdown,
	)
}

func escapeYAML(s string) string {
	return fmt.Sprintf("%q", s)
}

func promptFilename(prompt Prompt) string {
	stamp := prompt.Timestamp.UTC().Format("20060102T150405Z")
	return filepath.Join(promptsDirName, stamp+"-"+slugify(prompt.Title)+".md")
}

func savePrompt(prompt Prompt) (string, error) {
	_, promptsDir, _, err := projectPaths()
	if err != nil {
		return "", err
	}

	targetPath := filepath.Join(filepath.Dir(promptsDir), promptFilename(prompt))
	if err := os.WriteFile(targetPath, []byte(formatPromptFile(prompt)), 0o644); err != nil {
		return "", err
	}
	return targetPath, nil
}

func sortPrompts(prompts []Prompt) {
	sort.Slice(prompts, func(i, j int) bool {
		return prompts[i].Timestamp.Before(prompts[j].Timestamp)
	})
}
