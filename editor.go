package main

import (
	"errors"
	"os"
	"os/exec"
)

func editDraft(path string) error {
	editor, err := preferredEditor()
	if err != nil {
		return err
	}

	cmd := exec.Command(editor, path)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func preferredEditor() (string, error) {
	if _, err := exec.LookPath("vi"); err == nil {
		return "vi", nil
	}

	if editor := os.Getenv("EDITOR"); editor != "" {
		if _, err := exec.LookPath(editor); err == nil {
			return editor, nil
		}
	}

	return "", errors.New("could not find `vi` or an editor from $EDITOR")
}
