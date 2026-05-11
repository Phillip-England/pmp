---
title: Removed internal terminal
timestamp: 2026-05-09T20:25:00Z
---

# Removed internal terminal

Removed the integrated terminal feature from PMP. Deleted all terminal-related code: handlers (serveTerminal, serveTerminalWS, serveTerminalSessions), types (terminalSession, terminalManager, etc.), route handlers, navigation item, CSS, and `terminalTemplate`. Deleted `ui/xterm.js`, `ui/xterm.css`, `ui/xterm-addon-fit.js`, and `assets.go`. Kept `websocketAccept` and `writeWebsocketTextFrame` as they're used by the project file watcher. Remote SSH terminal in `lead/` was not touched.
