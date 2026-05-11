# pmp

`pmp` is a chronological prompt memory tool for local projects.

It stores prompts as markdown files, keeps lightweight project memory alongside them, and compiles selected history into a handoff document that another model or collaborator can use later. The design is intentionally simple: prompts matter in the order they happened, not in a nested hierarchy.

## What It Does

- Tracks prompts in `.pmp/prompts/` as timestamped markdown files with YAML frontmatter
- Uses built-in compile instructions by default
- Stores reusable project memory in `.pmp/memory/`
- Stores assistant history notes in `.pmp/history/`
- Lets you mark a prompt and compile from that mark forward
- Supports a focused CLI for fast operations
- Provides a local web UI for browsing project context and compiling history

## Philosophy

Most prompt tooling leans hard on trees, chats, or opaque state. `pmp` is closer to a timestamped project journal:

- Prompts are primary records
- Order is more important than hierarchy
- Everything important is plain text on disk
- Compiled output should be useful outside the app

## Installation

Build from source with Go:

```bash
go build -o pmp .
```

Or run it directly:

```bash
go run .
```

Module path:

```text
github.com/phillip-england/pmp
```

## Quick Start

Initialize a project:

```bash
pmp init
```

Open the web UI for the current project:

```bash
pmp
```

That default command auto-initializes the project if needed, starts the local server on `127.0.0.1:8765`, and opens the browser.

Save a prompt from the CLI:

```bash
pmp new "Add memory support" "We need a memory section in the compiled output."
```

List prompts:

```bash
pmp list
```

Compile everything to stdout:

```bash
pmp compile --stdout
```

## Project Layout

Running `pmp init` creates and maintains this structure:

```text
your-project/
├── .pmp/
│   ├── draft.md
│   ├── marks.txt
│   ├── memory/
│   ├── prompts/
│   └── history/
```

What each part is for:

- `.pmp/prompts/`: chronological prompt history
- `.pmp/memory/`: durable project context that should apply across prompts
- `.pmp/history/`: assistant history notes written after important work completes
- `.pmp/marks.txt`: the current marked prompt index
- `.pmp/draft.md`: draft buffer for editor-based prompt entry

## CLI Reference

The CLI is intentionally narrow. It is meant for fast capture and simple maintenance, not for exposing every UI action.

### `pmp`

Starts the web UI for the current project. If the project is not initialized yet, it initializes it first.

### `pmp init`

Initializes `.pmp/` storage in the current directory.

### `pmp new <title> <body>`

Creates a new prompt from the command line.

Example:

```bash
pmp new "Fix compile output" "Include memory before skills and prompts."
```

### `pmp list`

Lists prompts newest first. Marked prompts are shown with `*`.

### `pmp mark <index> [<index> ...]`

Marks one or more prompt indexes.

Examples:

```bash
pmp mark 12
pmp mark 12 13 14
pmp mark clear
```

### `pmp unmark <index> [<index> ...]`

Removes marks for specific prompt indexes.

### `pmp delete <index>`
### `pmp delete <start> <end>`

Deletes one prompt or an inclusive range of prompts.

Examples:

```bash
pmp delete 8
pmp delete 8 12
```

### `pmp compile`

Compiles prompt history into a structured document containing:

1. Instructions
2. Memory
3. Selected skills
4. Selected prompts

By default, `pmp compile` copies the compiled result to the system clipboard.

Useful flags:

- `--stdout`: write compiled output to stdout instead of the clipboard
- `--output <file>`: write compiled output to a file
- `--from-mark`: compile prompts after the current mark
- `--range <start> <end>`: compile an inclusive prompt range
- `--skill <name>`: include a named skill
- `--skills name-a,name-b`: include multiple skills
- `--include-instructions=false`: omit the built-in instructions section
- `--update-mark=false`: do not move the mark after compile

Examples:

```bash
pmp compile --stdout
pmp compile --from-mark --stdout
pmp compile --range 4 9 --output ./compiled.md
pmp compile --from-mark --skill ui-notes --skill release-checklist --stdout
pmp compile --stdout --include-instructions=false
```

## How Marking Works

Marks are used as a moving checkpoint.

- A project starts by ensuring the first saved prompt becomes the initial mark
- Compiling from mark skips the marked prompt and includes only newer prompts
- Compile operations update the mark by default
- You can disable that behavior with `--update-mark=false`

This makes it practical to compile only the prompts that matter since the last major handoff.

## Web UI

The browser UI is where the broader project workflow lives. The CLI covers quick capture and simple operations; the UI covers the richer context management.

### New

Create a prompt with a title and body in a simple form.

### Prompts

The prompts page is the main browsing and compile surface.

It supports:

- viewing prompts newest first
- filtering by search text
- filtering by date range
- seeing prompt indexes and the current mark
- deleting prompts
- compiling all prompts
- compiling after the mark
- compiling an inclusive range
- optionally including selected skills in the compilation

When you compile from the UI, the result is copied to the clipboard.

### Memory

Stores persistent project-specific context in `.pmp/memory/`.

Use memory for facts that should survive across many prompts, such as:

- product rules
- architecture assumptions
- recurring constraints
- important decisions worth carrying forward

Memory is included in compiled output ahead of skills and prompts.

### Skills

Stores optional reusable skill documents in the user config area, not in the project directory.

Skills are opt-in during compilation. PMP also seeds a few built-in defaults. They are useful for reusable guidance such as:

- coding conventions
- deployment checklists
- review heuristics
- writing styles

### History

Shows assistant history notes stored in `.pmp/history/`.

These are important because compiled instructions explicitly require the receiving model to write at least one history note after important work completes.

### Projects

Tracks known `pmp` projects and lets you switch between them in the UI.

Project discovery uses configurable scan roots plus a local registry of opened projects.

### Settings

Current settings include:

- accent color
- secondary accent color
- built-in theme presets
- project scan roots

Settings are stored in the user config directory.

## Config Files

`pmp` uses user-level config storage for global settings and the project registry.

By default it uses your OS config directory under `pmp/`. You can override that root with:

```bash
PMP_CONFIG_HOME=/custom/path
```

Global data currently includes:

- `settings.json`
- `projects.json`
- system-wide skill markdown files

## Prompt File Format

Prompts are plain markdown files with frontmatter:

```md
---
title: "Add compile ranges"
timestamp: 2026-05-10T00:09:55Z
---

# Add compile ranges

We need to compile a specific inclusive range from the CLI.
```

Titles are required, and prompt bodies cannot be empty.

## History Notes

Compiled output instructs downstream agents to write history notes back into the project. Those notes should:

- be new timestamped markdown files
- live in `.pmp/history/`
- use YAML frontmatter with `title` and `timestamp`
- include a matching top-level heading
- stay short when possible
- record the most important result, risk, blocker, or follow-up

This creates a small audit trail of what happened after a compilation was used.

## Why The CLI Is Limited

Not every action belongs in the terminal.

The current split is deliberate:

- CLI: quick prompt capture, listing, marking, deleting, compiling
- Web UI: browsing, filtering, memory management, skill selection, history, settings, multi-project navigation

That keeps the command surface practical while preserving a better interface for context-heavy tasks.

## Current State

This repository is plain Go and keeps data in readable files. That makes it easy to inspect, back up, version, and extend without depending on a database or external service.
