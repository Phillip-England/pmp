---
title: "Terminal first-load fix"
timestamp: "2026-05-09T18:51:04Z"
---

# Terminal first-load fix

Updated `/terminal` so opening the page creates the first terminal session server-side instead of waiting for a client-side action. The page now initializes the terminal only after the host is visible, which avoids the blank first-load state. Remaining caveat: if CDN terminal assets fail to load, the page now shows a clear UI failure message instead of silently appearing empty.
