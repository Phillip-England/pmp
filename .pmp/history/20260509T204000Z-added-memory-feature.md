---
title: Added Memory feature to pmp
timestamp: 2026-05-09T20:40:00Z
---

# Added Memory feature to pmp

Added a new Memory feature that is project-specific and included in every compilation. Changes:

- `project.go`: Added `MEMORY.md` file, `loadProjectMemory()`, `saveProjectMemory()`, and auto-creation during init
- `compile.go`: Memory now appears between Instructions and Skills in compiled output
- `serve.go`: Added `/memory` page, handler, nav item, and template
- `INSTRUCTIONS.md` default: Updated to explain the four-section structure (instructions, memory, skills, prompts)
