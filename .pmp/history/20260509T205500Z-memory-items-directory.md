---
title: Memory items now stored in .pmp/memory/ directory
timestamp: 2026-05-09T20:55:00Z
---

# Memory items now stored in .pmp/memory/ directory

Refactored Memory from a single `MEMORY.md` file to a directory-based system stored in `.pmp/memory/`. Each memory item is now a separate markdown file with frontmatter (title, timestamp).

- Created `Memory` struct with load/save/delete CRUD functions
- Added `/memory/api` endpoint for REST operations (GET list, POST create, PUT update, DELETE)
- Reworked `/memory` page: flex grid display, view/edit modal dialogs, add form
- Updated compile to include all memory items between instructions and skills
- Removed single-file `MEMORY.md` (old projects can manually migrate if needed)
