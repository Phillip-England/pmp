---
title: "Terminal page and instructions cleanup"
timestamp: "2026-05-09T18:47:10Z"
---

# Terminal page and instructions cleanup

Added a `/terminal` page with persistent server-managed shell sessions, tab switching, and `Cmd+T` or `Ctrl+T` terminal creation. Removed the stale `PROJECT.md` document so the repo now points at generic `INSTRUCTIONS.md` guidance instead. Main caveat: terminal sessions persist only while `pmp serve` is running, and the browser terminal depends on CDN-hosted `xterm.js` assets.
