---
title: Terminal jargon and tab persistence fix
timestamp: 2026-05-09T20:15:00Z
---

# Terminal jargon and tab persistence fix

Fixed two terminal issues:

1. **Jargon/garbled output**: The `decodeBase64` function was creating a new `TextDecoder()` for each chunk, which loses UTF-8 state across boundaries. Fixed by using a persistent decoder with `{stream: true}` mode (matching the working `lead` implementation).

2. **Tab state not persisted**: Active terminal tab was lost on page refresh. Fixed by storing the active tab ID in `localStorage` under key `pmp-terminal-active`.
