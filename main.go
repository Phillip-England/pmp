package main

import (
	"errors"
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "pmp:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return runDefault()
	}

	switch args[0] {
	case "init":
		if len(args) != 1 {
			return errors.New("`pmp init` does not accept arguments")
		}
		return runInit()
	case "serve":
		if len(args) != 1 {
			return errors.New("`pmp serve` does not accept arguments")
		}
		return runServe()
	case "list":
		return runListCommand(args[1:])
	case "new":
		return runNewCommand(args[1:])
	case "add":
		return runAddCommand(args[1:])
	case "mark":
		return runMark(args[1:])
	case "unmark":
		return runUnmark(args[1:])
	case "delete":
		return runDeleteCommand(args[1:])
	case "remove":
		return runRemoveCommand(args[1:])
	case "print":
		return runPrintCommand(args[1:])
	case "compile":
		return runCompileCommand(args[1:])
	case "-h", "--help", "help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage() {
	fmt.Println(`pmp tracks prompts in chronological order.

Usage:
  pmp         Auto-initialize and open the web UI on the new prompt page
  pmp init    Initialize prompt storage in the current directory
  pmp new     Save a prompt from the CLI with a title and body
  pmp add     Add a prompt, skill, or memory from the CLI
  pmp serve   Serve the browser UI for browsing and compiling prompts
  pmp list    List prompts, history, skills, or memories
  pmp print   Print one prompt, history, skill, or memory
  pmp mark    Mark one prompt index for tracking
  pmp unmark  Remove prompt marks
  pmp delete  Delete prompts by index or inclusive range
  pmp remove  Remove one prompt, history, skill, or memory
  pmp compile Compile prompt history or assistant history from the CLI

Compile flags:
  history
  --from-mark
  --range <start> <end>
  --stdout
  --output <file>
  --skill <name>
  --skills <name-a,name-b>
  --include-instructions=false
  --update-mark=false`)
}
