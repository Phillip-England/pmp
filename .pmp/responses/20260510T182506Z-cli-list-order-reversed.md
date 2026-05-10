---
title: "CLI List Order Reversed"
timestamp: "2026-05-10T18:25:06Z"
---

# CLI List Order Reversed

The default `pmp list` prompt output now prints oldest to newest, so the newest prompt lands at the bottom and is most visible in the terminal. This was limited to the CLI prompt listing path and covered with a regression test to avoid changing browser ordering by accident.
