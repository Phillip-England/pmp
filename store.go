package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runNewCommand(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: `pmp new <title> <body>`")
	}

	prompt, err := parsePromptFields(args[0], args[1])
	if err != nil {
		return err
	}

	targetPath, err := savePrompt(prompt)
	if err != nil {
		return err
	}
	if err := ensureInitialPromptMark(); err != nil {
		return err
	}

	_, _ = os.Stdout.WriteString("saved " + filepath.Base(targetPath) + "\n")
	return nil
}

func runPrompt() error {
	if err := ensureProject(); err != nil {
		return err
	}

	prompts, err := loadPrompts()
	if err != nil {
		return err
	}
	fmt.Printf("project prompts: %d\nwriting prompt: %d\n", len(prompts), len(prompts)+1)

	_, _, draftPath, err := projectPaths()
	if err != nil {
		return err
	}

	if err := editDraft(draftPath); err != nil {
		return err
	}

	markdownBytes, err := os.ReadFile(draftPath)
	if err != nil {
		return err
	}

	prompt, err := parseDraft(string(markdownBytes))
	if err != nil {
		return err
	}

	targetPath, err := savePrompt(prompt)
	if err != nil {
		return err
	}
	if err := ensureInitialPromptMark(); err != nil {
		return err
	}
	if err := os.WriteFile(draftPath, nil, 0o644); err != nil {
		return err
	}

	_, _ = os.Stdout.WriteString("saved " + filepath.Base(targetPath) + "\n")
	return nil
}

func runDefault() error {
	if err := ensureProjectInitialized(); err != nil {
		return err
	}
	return runServe()
}

func loadPrompts() ([]Prompt, error) {
	if err := ensureProject(); err != nil {
		return nil, err
	}
	_, promptsDir, _, err := projectPaths()
	if err != nil {
		return nil, err
	}
	return loadMarkdownRecords(promptsDir)
}

func loadHistory() ([]Prompt, error) {
	if err := ensureProject(); err != nil {
		return nil, err
	}
	if err := migrateLegacyResponsesToHistory(); err != nil {
		return nil, err
	}
	historyDir, err := historyPath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(historyDir, 0o755); err != nil {
		return nil, err
	}
	return loadMarkdownRecords(historyDir)
}

func loadResponses() ([]Prompt, error) {
	return loadHistory()
}

func loadMarkdownRecords(dir string) ([]Prompt, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	prompts := make([]Prompt, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		prompt, err := readPromptFile(path)
		if err != nil {
			return nil, err
		}
		prompts = append(prompts, prompt)
	}
	sortPrompts(prompts)
	return prompts, nil
}

func readPromptFile(path string) (Prompt, error) {
	file, err := os.Open(path)
	if err != nil {
		return Prompt{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return Prompt{}, errors.New("prompt file is empty")
	}
	if scanner.Text() != "---" {
		return Prompt{}, errors.New("prompt file is missing frontmatter")
	}

	meta := map[string]string{}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "---" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return Prompt{}, errors.New("invalid frontmatter line")
		}
		meta[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	if err := scanner.Err(); err != nil {
		return Prompt{}, err
	}

	var bodyLines []string
	for scanner.Scan() {
		bodyLines = append(bodyLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return Prompt{}, err
	}

	timestamp, err := time.Parse(time.RFC3339, strings.Trim(meta["timestamp"], `"`))
	if err != nil {
		return Prompt{}, err
	}

	title := strings.Trim(meta["title"], `"`)
	return Prompt{
		Title:     title,
		Timestamp: timestamp,
		Markdown:  strings.TrimLeft(strings.Join(bodyLines, "\n"), "\n"),
		Path:      path,
	}, nil
}

func runList() error {
	prompts, err := loadPrompts()
	if err != nil {
		return err
	}
	marks, err := loadMarks()
	if err != nil {
		return err
	}

	if len(prompts) == 0 {
		fmt.Println("no prompts yet")
		return nil
	}

	for i := 0; i < len(prompts); i++ {
		prompt := prompts[i]
		indexLabel := fmt.Sprintf("[%d]", i)
		if marks[i] {
			indexLabel = fmt.Sprintf("[%d*]", i)
		}
		fmt.Printf("%s %s  %s\n", indexLabel, prompt.Timestamp.Local().Format("2006-01-02 15:04:05 MST"), prompt.Title)
	}
	return nil
}
