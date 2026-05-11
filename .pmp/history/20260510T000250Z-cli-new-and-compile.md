---
title: "CLI New And Compile Commands"
timestamp: 2026-05-10T00:02:50Z
---

# CLI New And Compile Commands

Added a narrow CLI path for non-UI workflows: `pmp new <title> <body>` now saves prompts directly and preserves the initial mark behavior, and `pmp compile` now supports `--from-mark`, `--range <start> <end>`, `--update-mark=false`, `--skill`, `--skills`, `--output`, and `--stdout`. Verified with `go test ./...`.
