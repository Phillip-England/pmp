---
title: "Compile After Mark"
timestamp: "2026-05-10T00:18:29Z"
---

# Compile After Mark

Changed `compile --from-mark` and the UI `from mark` mode to skip the marked prompt and include only newer prompts. Updated the selection helper, tests, and README/UI wording to match. Risk: existing users who relied on inclusive behavior will now get one fewer prompt in mark-based compilations.
