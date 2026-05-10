---
title: "CLI record controls"
timestamp: 2026-05-10T00:31:46Z
---

# CLI record controls

Added CLI `add`, `remove`, and `print` commands for prompts, skills, and memories, and extended `list` to show skills or memories. Prompt marking now keeps a single active mark at a time, matching compile-after-mark behavior. Risk: memory lookup uses a slug derived from the title, so older duplicate titles that normalize to the same slug will need renaming before `print/remove memory` can be used unambiguously.
