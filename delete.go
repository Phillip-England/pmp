package main

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

type deleteRange struct {
	Start int
	End   int
}

func parseDeleteArgs(args []string) (deleteRange, error) {
	switch len(args) {
	case 1:
		index, err := strconv.Atoi(args[0])
		if err != nil {
			return deleteRange{}, fmt.Errorf("invalid prompt index %q", args[0])
		}
		return deleteRange{Start: index, End: index}, nil
	case 2:
		start, err := strconv.Atoi(args[0])
		if err != nil {
			return deleteRange{}, fmt.Errorf("invalid start index %q", args[0])
		}
		end, err := strconv.Atoi(args[1])
		if err != nil {
			return deleteRange{}, fmt.Errorf("invalid end index %q", args[1])
		}
		return deleteRange{Start: start, End: end}, nil
	default:
		return deleteRange{}, errors.New("usage: `pmp delete <index>` or `pmp delete <start> <end>`")
	}
}

func runDeleteCommand(args []string) error {
	rng, err := parseDeleteArgs(args)
	if err != nil {
		return err
	}
	return runDelete(rng)
}

func runDelete(rng deleteRange) error {
	prompts, err := loadPrompts()
	if err != nil {
		return err
	}
	if len(prompts) == 0 {
		return errors.New("no prompts to delete")
	}
	if rng.Start < 0 || rng.End < 0 {
		return errors.New("delete indexes must be non-negative")
	}
	if rng.Start > rng.End {
		return errors.New("delete start index must be less than or equal to end index")
	}
	if rng.End >= len(prompts) {
		return fmt.Errorf("delete range out of bounds; highest prompt index is %d", len(prompts)-1)
	}

	for i := rng.Start; i <= rng.End; i++ {
		if err := os.Remove(prompts[i].Path); err != nil {
			return err
		}
	}

	marks, err := loadMarks()
	if err != nil {
		return err
	}
	updated := reindexMarksAfterDelete(marks, rng)
	if err := saveMarks(updated); err != nil {
		return err
	}

	if rng.Start == rng.End {
		fmt.Printf("deleted prompt %d\n", rng.Start)
	} else {
		fmt.Printf("deleted prompts %d through %d\n", rng.Start, rng.End)
	}
	return nil
}

func reindexMarksAfterDelete(marks map[int]bool, rng deleteRange) map[int]bool {
	updated := map[int]bool{}
	shift := rng.End - rng.Start + 1
	for index := range marks {
		switch {
		case index < rng.Start:
			updated[index] = true
		case index > rng.End:
			updated[index-shift] = true
		}
	}
	return updated
}
