---
title: "Memory Multipart And Initial Mark Fixes"
timestamp: 2026-05-09T23:51:41Z
---

# Memory Multipart And Initial Mark Fixes

Fixed two regressions: memory create/edit now accept multipart form submissions reliably, and the first saved prompt now establishes the initial mark automatically when no marks exist. Also corrected memory edits so a title change replaces the old file instead of leaving a stale duplicate behind. Verified with `go test ./...`.
