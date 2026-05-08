# PROJECT

This repository contains `pmp`, a small Go CLI and browser UI for storing prompts in chronological order.

## What this project does

`pmp` helps track the full prompt history of a project over time. The core assumption is that chronological order matters more than hierarchy when reconstructing how a project evolved.

## Important files

- `main.go` wires the CLI commands.
- `prompt.go` validates, formats, and sorts prompt records.
- `project.go` manages `.pmp` project initialization and storage paths.
- `compile.go` compiles prompt history into a single string for clipboard or file output.
- `serve.go` serves the local browser UI for browsing prompts and compiling selections.
- `SPEC.md` contains the product description and behavioral intent.

## Runtime layout

When a user runs `pmp init` in a directory, the tool creates:

- `.pmp/prompts/` for stored markdown prompt files
- `.pmp/draft.md` for the current draft
- `.pmp/marks.txt` for marked prompt indexes
- `PROJECT.md` as a concise note for humans and language models

## Prompt format

Each prompt is stored as markdown with YAML frontmatter. The frontmatter currently includes `title` and `timestamp`. The body is the original markdown prompt text, which must include a top-level markdown heading and body content.
