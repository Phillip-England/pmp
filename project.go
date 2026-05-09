package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	projectDirName      = ".pmp"
	promptsDirName      = "prompts"
	draftFileName       = "draft.md"
	marksFileName       = "marks.txt"
	projectNoteFileName = "PROJECT.md"
)

const projectNoteContents = `# PROJECT

This directory contains a Prompt Memory Project managed by ` + "`pmp`" + `.

## Purpose

The tool stores prompts in chronological order so the full history of a project can be reconstructed later.

## Layout

- ` + "`.pmp/prompts/`" + ` contains saved prompt files as markdown with frontmatter.
- ` + "`.pmp/marks.txt`" + ` stores marked prompt indexes used by the CLI today.
- system-wide skills are stored in the user config directory for ` + "`pmp`" + ` and can be opted into during compilation.
- system settings such as theme accent and project scan roots are stored in the user config directory for ` + "`pmp`" + `.

## Prompt format

Each prompt file is markdown with frontmatter metadata:

- ` + "`title`" + `
- ` + "`timestamp`" + `

The body contains the original prompt text and should begin with a markdown heading.

## Key commands

- ` + "`pmp`" + ` auto-initializes the project if needed and opens the web UI on the new prompt page.
- ` + "`pmp list`" + ` prints prompts newest first.
- ` + "`pmp compile`" + ` compiles prompt history to the clipboard or a file.
- ` + "`pmp serve`" + ` opens the local browser UI for browsing and compiling prompts.
`

var projectRootState struct {
	mu       sync.RWMutex
	override string
}

func projectRoot() (string, error) {
	projectRootState.mu.RLock()
	override := projectRootState.override
	projectRootState.mu.RUnlock()
	if strings.TrimSpace(override) != "" {
		return override, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return wd, nil
}

func setProjectRootOverride(root string) error {
	absRoot, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return err
	}
	projectRootState.mu.Lock()
	projectRootState.override = filepath.Clean(absRoot)
	projectRootState.mu.Unlock()
	return nil
}

func clearProjectRootOverride() {
	projectRootState.mu.Lock()
	projectRootState.override = ""
	projectRootState.mu.Unlock()
}

func projectPaths() (base string, prompts string, draft string, err error) {
	root, err := projectRoot()
	if err != nil {
		return "", "", "", err
	}
	base = filepath.Join(root, projectDirName)
	prompts = filepath.Join(base, promptsDirName)
	draft = filepath.Join(base, draftFileName)
	return base, prompts, draft, nil
}

func runInit() error {
	root, err := projectRoot()
	if err != nil {
		return err
	}
	base, prompts, draft, err := projectPaths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(prompts, 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(draft); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(draft, nil, 0o644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	marks := filepath.Join(base, marksFileName)
	if _, err := os.Stat(marks); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(marks, nil, 0o644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if _, err := ensureSkillsStorage(); err != nil {
		return err
	}
	projectNotePath := filepath.Join(root, projectNoteFileName)
	if _, err := os.Stat(projectNotePath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(projectNotePath, []byte(projectNoteContents), 0o644); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err := registerProject(root); err != nil {
		return err
	}
	_, _ = os.Stdout.WriteString("initialized " + base + "\n")
	return nil
}

func ensureProjectInitialized() error {
	if err := ensureProject(); err != nil {
		if err := runInit(); err != nil {
			return err
		}
	}
	return nil
}

func ensureProject() error {
	root, err := projectRoot()
	if err != nil {
		return err
	}
	return ensureProjectAtRoot(root)
}

func ensureProjectAtRoot(root string) error {
	base := filepath.Join(root, projectDirName)
	prompts := filepath.Join(base, promptsDirName)
	if _, err := os.Stat(base); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("project not initialized; run `pmp init` first")
		}
		return err
	}
	if _, err := os.Stat(prompts); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.New("project storage is incomplete; run `pmp init` again")
		}
		return err
	}
	return nil
}

func marksPath() (string, error) {
	base, _, _, err := projectPaths()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, marksFileName), nil
}
