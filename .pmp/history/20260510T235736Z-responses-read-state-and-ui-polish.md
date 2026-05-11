---
title: "Responses Read State And UI Polish"
timestamp: "2026-05-10T23:57:36Z"
---

# Responses Read State And UI Polish

Responses now flip to read immediately in the UI when opened, including the unread badge and unread count, and the page also has a new Mark all as read action. I also compacted the color pickers in settings, colored the Quick Compile button, and added a stronger hover treatment for existing skill cards. Follow-up risk: unread state still keys off response file paths, so any later rename flow should preserve that mapping.
