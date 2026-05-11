---
title: "Local xterm assets for terminal rendering"
timestamp: "2026-05-09T19:42:31Z"
---

# Local xterm assets for terminal rendering

Compared `pmp` against `lead` and fixed the main mismatch: `pmp` was falling back to plain text because it was not serving a local terminal emulator. Added bundled `xterm` assets and switched new PTY sessions back to `TERM=xterm-256color`, so escape sequences render in the terminal instead of appearing as jargon. The plain-text fallback still exists, but it should only be used if local JS fails unexpectedly.
