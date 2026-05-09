---
title: "Terminal fallback renderer"
timestamp: "2026-05-09T18:53:47Z"
---

# Terminal fallback renderer

Reworked the terminal page so it no longer depends on CDN-hosted `xterm.js` to appear. If the enhanced terminal library is unavailable, PMP now falls back to a built-in terminal renderer that still shows output and forwards keyboard input to the server PTY. Also set `TERM=dumb` for new sessions to reduce escape-sequence noise in the fallback view.
