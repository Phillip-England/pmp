package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func runListCommand(args []string) error {
	switch len(args) {
	case 0:
		return runList()
	case 1:
		switch strings.ToLower(strings.TrimSpace(args[0])) {
		case "prompt", "prompts":
			return runList()
		case "history", "histories", "response", "responses":
			return runListHistory()
		case "skill", "skills":
			return runListSkills()
		case "memory", "memories":
			return runListMemories()
		default:
			return fmt.Errorf("unknown list target %q", args[0])
		}
	default:
		return errors.New("usage: `pmp list [prompts|history|skills|memories]`")
	}
}

func runAddCommand(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: `pmp add prompt <title> <body>`, `pmp add skill <name> <body>`, or `pmp add memory <title> <body>`")
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "prompt":
		return runNewCommand(args[1:])
	case "skill":
		return runAddSkill(args[1:])
	case "memory":
		return runAddMemory(args[1:])
	default:
		return fmt.Errorf("unknown add target %q", args[0])
	}
}

func runRemoveCommand(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: `pmp remove prompt <index> [<end>]`, `pmp remove history <index>`, `pmp remove skill <name>`, or `pmp remove memory <name>`")
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "prompt":
		return runDeleteCommand(args[1:])
	case "history", "response":
		return runRemoveHistory(args[1:])
	case "skill":
		return runRemoveSkill(args[1:])
	case "memory":
		return runRemoveMemory(args[1:])
	default:
		return fmt.Errorf("unknown remove target %q", args[0])
	}
}

func runPrintCommand(args []string) error {
	if len(args) < 1 {
		return errors.New("usage: `pmp print prompt <index>`, `pmp print history <index>`, `pmp print skill <name>`, or `pmp print memory <name>`")
	}

	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "prompt":
		return runPrintPrompt(args[1:])
	case "history", "response":
		return runPrintHistory(args[1:])
	case "skill":
		return runPrintSkill(args[1:])
	case "memory":
		return runPrintMemory(args[1:])
	default:
		return fmt.Errorf("unknown print target %q", args[0])
	}
}

func runListSkills() error {
	skills, err := loadSkills()
	if err != nil {
		return err
	}
	if len(skills) == 0 {
		fmt.Println("no skills yet")
		return nil
	}
	for _, skill := range skills {
		fmt.Printf("[%s]\n", skill.Name)
	}
	return nil
}

func runListHistory() error {
	history, err := loadHistory()
	if err != nil {
		return err
	}
	if len(history) == 0 {
		fmt.Println("no history yet")
		return nil
	}
	for i := len(history) - 1; i >= 0; i-- {
		entry := history[i]
		fmt.Printf("[%d] %s  %s\n", i, entry.Timestamp.Local().Format("2006-01-02 15:04:05 MST"), entry.Title)
	}
	return nil
}

func runListMemories() error {
	memories, err := loadMemories()
	if err != nil {
		return err
	}
	if len(memories) == 0 {
		fmt.Println("no memories yet")
		return nil
	}
	for _, memory := range memories {
		fmt.Printf("[%s] %s  %s\n", memoryLookupName(memory), memory.Timestamp.Local().Format("2006-01-02 15:04:05 MST"), memory.Title)
	}
	return nil
}

func runAddSkill(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: `pmp add skill <name> <body>`")
	}
	name := normalizeSkillName(args[0])
	if err := validateSkillName(name); err != nil {
		return err
	}
	existing, err := loadSkills()
	if err != nil {
		return err
	}
	for _, skill := range existing {
		if normalizeSkillName(skill.Name) == name {
			return fmt.Errorf("skill %q already exists", name)
		}
	}
	if err := saveSkill(name, args[1]); err != nil {
		return err
	}
	fmt.Printf("saved skill %s\n", name)
	return nil
}

func runAddMemory(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: `pmp add memory <title> <body>`")
	}

	title := strings.TrimSpace(args[0])
	body := strings.TrimSpace(args[1])
	if title == "" {
		return errors.New("memory title cannot be empty")
	}
	if body == "" {
		return errors.New("memory body cannot be empty")
	}

	name := normalizeMemoryName(title)
	if name == "" {
		return errors.New("memory title must include letters or numbers")
	}
	if _, err := findMemoryByName(name); err == nil {
		return fmt.Errorf("memory %q already exists", name)
	} else if !errors.Is(err, errMemoryNotFound) {
		return err
	}

	path, err := saveMemory(Memory{
		Title:     title,
		Timestamp: time.Now().UTC(),
		Body:      body,
	})
	if err != nil {
		return err
	}
	fmt.Printf("saved memory %s (%s)\n", name, filepathBase(path))
	return nil
}

func runRemoveSkill(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: `pmp remove skill <name>`")
	}
	skill, err := findSkillByName(args[0])
	if err != nil {
		return err
	}
	if err := deleteSkill(skill.Name); err != nil {
		return err
	}
	fmt.Printf("removed skill %s\n", skill.Name)
	return nil
}

func runRemoveHistory(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: `pmp remove history <index>`")
	}
	entry, index, err := historyByIndexArg(args[0])
	if err != nil {
		return err
	}
	if err := os.Remove(entry.Path); err != nil {
		return err
	}
	fmt.Printf("removed history %d\n", index)
	return nil
}

func runRemoveMemory(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: `pmp remove memory <name>`")
	}
	memory, err := findMemoryByName(args[0])
	if err != nil {
		return err
	}
	if err := deleteMemory(memory.Path); err != nil {
		return err
	}
	fmt.Printf("removed memory %s\n", memoryLookupName(memory))
	return nil
}

func runPrintPrompt(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: `pmp print prompt <index>`")
	}
	prompt, _, err := promptByIndexArg(args[0])
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(strings.TrimRight(prompt.Markdown, "\n") + "\n")
	return err
}

func runPrintSkill(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: `pmp print skill <name>`")
	}
	skill, err := findSkillByName(args[0])
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(strings.TrimRight(skill.Body, "\n") + "\n")
	return err
}

func runPrintHistory(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: `pmp print history <index>`")
	}
	entry, _, err := historyByIndexArg(args[0])
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(strings.TrimRight(entry.Markdown, "\n") + "\n")
	return err
}

func runListResponses() error {
	return runListHistory()
}

func runRemoveResponse(args []string) error {
	return runRemoveHistory(args)
}

func runPrintResponse(args []string) error {
	return runPrintHistory(args)
}

func runPrintMemory(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: `pmp print memory <name>`")
	}
	memory, err := findMemoryByName(args[0])
	if err != nil {
		return err
	}
	_, err = os.Stdout.WriteString(formatMemoryDisplay(memory))
	return err
}

var errMemoryNotFound = errors.New("memory not found")
var errSkillNotFound = errors.New("skill not found")

func promptByIndexArg(raw string) (Prompt, int, error) {
	prompts, err := loadPrompts()
	if err != nil {
		return Prompt{}, 0, err
	}
	index, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return Prompt{}, 0, fmt.Errorf("invalid prompt index %q", raw)
	}
	if index < 0 || index >= len(prompts) {
		return Prompt{}, 0, fmt.Errorf("prompt index %d is out of bounds; highest prompt index is %d", index, len(prompts)-1)
	}
	return prompts[index], index, nil
}

func historyByIndexArg(raw string) (Prompt, int, error) {
	history, err := loadHistory()
	if err != nil {
		return Prompt{}, 0, err
	}
	index, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return Prompt{}, 0, fmt.Errorf("invalid history index %q", raw)
	}
	if index < 0 || index >= len(history) {
		return Prompt{}, 0, fmt.Errorf("history index %d is out of bounds; highest history index is %d", index, len(history)-1)
	}
	return history[index], index, nil
}

func responseByIndexArg(raw string) (Prompt, int, error) {
	return historyByIndexArg(raw)
}

func findSkillByName(name string) (Skill, error) {
	target := normalizeSkillName(name)
	if err := validateSkillName(target); err != nil {
		return Skill{}, err
	}
	skills, err := loadSkills()
	if err != nil {
		return Skill{}, err
	}
	for _, skill := range skills {
		if normalizeSkillName(skill.Name) == target {
			return skill, nil
		}
	}
	return Skill{}, fmt.Errorf("%w: %s", errSkillNotFound, target)
}

func findMemoryByName(name string) (Memory, error) {
	target := normalizeMemoryName(name)
	if target == "" {
		return Memory{}, fmt.Errorf("%w: %s", errMemoryNotFound, strings.TrimSpace(name))
	}
	memories, err := loadMemories()
	if err != nil {
		return Memory{}, err
	}
	var match *Memory
	for i := range memories {
		if memoryLookupName(memories[i]) != target {
			continue
		}
		if match != nil {
			return Memory{}, fmt.Errorf("memory name %q is ambiguous; rename duplicates before using CLI print/remove", target)
		}
		match = &memories[i]
	}
	if match == nil {
		return Memory{}, fmt.Errorf("%w: %s", errMemoryNotFound, target)
	}
	return *match, nil
}

func normalizeMemoryName(name string) string {
	return slugify(name)
}

func memoryLookupName(memory Memory) string {
	return normalizeMemoryName(memory.Title)
}

func formatMemoryDisplay(memory Memory) string {
	return fmt.Sprintf("# %s\n\n%s\n", memory.Title, strings.TrimSpace(memory.Body))
}

func filepathBase(path string) string {
	parts := strings.Split(strings.ReplaceAll(path, "\\", "/"), "/")
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}
