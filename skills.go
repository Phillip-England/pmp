package main

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const skillsDirName = "skills"

type Skill struct {
	Name string
	Body string
}

func skillsPath() (string, error) {
	base, err := configRootDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, skillsDirName), nil
}

func ensureSkillsStorage() (string, error) {
	path, err := skillsPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	return path, nil
}

func normalizeSkillName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.Join(strings.Fields(name), "-")
	var b strings.Builder
	lastDash := false
	for _, r := range name {
		isAlphaNum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlphaNum {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' && !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func validateSkillName(name string) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("skill name cannot be empty")
	}
	if normalizeSkillName(name) != strings.TrimSpace(strings.ToLower(name)) {
		return errors.New("skill name must use lowercase letters, numbers, and dashes only")
	}
	return nil
}

func skillFilePath(name string) (string, error) {
	dir, err := ensureSkillsStorage()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, normalizeSkillName(name)+".md"), nil
}

func loadSkills() ([]Skill, error) {
	dir, err := ensureSkillsStorage()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	skills := make([]Skill, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		body, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		skills = append(skills, Skill{
			Name: name,
			Body: string(body),
		})
	}
	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})
	return skills, nil
}

func saveSkill(name, body string) error {
	name = normalizeSkillName(name)
	if err := validateSkillName(name); err != nil {
		return err
	}
	path, err := skillFilePath(name)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(body)+"\n"), 0o644)
}

func deleteSkill(name string) error {
	path, err := skillFilePath(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func skillNamesSet(names []string) map[string]bool {
	set := map[string]bool{}
	for _, name := range names {
		name = normalizeSkillName(name)
		if name == "" {
			continue
		}
		set[name] = true
	}
	return set
}

func selectedSkills(skills []Skill, included []string) []Skill {
	includedSet := skillNamesSet(included)
	selected := make([]Skill, 0, len(skills))
	for _, skill := range skills {
		if !includedSet[normalizeSkillName(skill.Name)] {
			continue
		}
		selected = append(selected, skill)
	}
	return selected
}
