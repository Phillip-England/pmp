---
title: "History Migration And Projects UI"
timestamp: "2026-05-11T00:24:03Z"
---

# History Migration And Projects UI

Migrated the assistant note feature from `responses` to `history`, added history compilation output and UI controls, made project cards directly clickable, and expanded the built-in theme presets. Backward compatibility is still in place for legacy `responses` files and routes so existing projects keep working during the transition. `go test ./...` passed.
