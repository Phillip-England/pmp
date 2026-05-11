---
title: "New Page And Responses UI"
timestamp: "2026-05-10T21:41:05Z"
---

# New Page And Responses UI

Implemented three UI changes in the web app: `/new` now shows prompt titles newer than the current mark and has a quick compile action, `/responses` now supports unread tracking plus multi-select delete, and `/skills` cards open directly without the extra button. Response read state now persists in `.pmp/response-reads.json`. Main follow-up risk: unread state is tracked by file path, so any future response file rename flow should preserve or migrate that mapping.
