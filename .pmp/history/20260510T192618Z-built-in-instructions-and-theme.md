---
title: "Built-in Instructions And Theme Settings"
timestamp: 2026-05-10T19:26:18Z
---

# Built-in Instructions And Theme Settings

Built compile instructions are now binary-owned, the instructions page is removed, and compile can exclude that section with `--include-instructions=false` or the prompts-page checkbox. Secondary accent color is now a global setting, the working directory banner only shows the path, and PMP seeds default skills automatically. Risk: existing custom `INSTRUCTIONS.md` files are ignored; only legacy `PROJECT.md` fallback remains.
