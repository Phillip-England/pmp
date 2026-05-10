package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func loadMarks() (map[int]bool, error) {
	if err := ensureProject(); err != nil {
		return nil, err
	}

	path, err := marksPath()
	if err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[int]bool{}, nil
		}
		return nil, err
	}
	defer file.Close()

	marks := map[int]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		index, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("invalid mark index %q", line)
		}
		marks[index] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return marks, nil
}

func saveMarks(marks map[int]bool) error {
	path, err := marksPath()
	if err != nil {
		return err
	}

	indexes := make([]int, 0, len(marks))
	for index := range marks {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)

	var b strings.Builder
	for _, index := range indexes {
		b.WriteString(strconv.Itoa(index))
		b.WriteByte('\n')
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func runMark(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: `pmp mark <index>` or `pmp mark clear`")
	}
	if len(args) == 1 && args[0] == "clear" {
		return clearMarks()
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: `pmp mark <index>` or `pmp mark clear`")
	}

	prompts, err := loadPrompts()
	if err != nil {
		return err
	}

	index, err := strconv.Atoi(args[0])
	if err != nil {
		return fmt.Errorf("invalid mark index %q", args[0])
	}
	if index < 0 || index >= len(prompts) {
		return fmt.Errorf("mark index %d is out of bounds; highest prompt index is %d", index, len(prompts)-1)
	}

	if err := saveMarks(map[int]bool{index: true}); err != nil {
		return err
	}
	fmt.Printf("marked prompt %d\n", index)
	return nil
}

func runUnmark(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: `pmp unmark <index> [<index> ...]`")
	}

	marks, err := loadMarks()
	if err != nil {
		return err
	}
	for _, arg := range args {
		index, err := strconv.Atoi(arg)
		if err != nil {
			return fmt.Errorf("invalid mark index %q", arg)
		}
		delete(marks, index)
	}
	if err := saveMarks(marks); err != nil {
		return err
	}
	fmt.Println("marks updated")
	return nil
}

func clearMarks() error {
	if err := saveMarks(map[int]bool{}); err != nil {
		return err
	}
	fmt.Println("marks cleared")
	return nil
}
