---
title: "Response click and delete"
timestamp: 2026-05-10T01:30:06Z
---

# Response click and delete

Responses now open from card click in the web UI, the extra View button is gone, and responses can be deleted from both the web UI and CLI. CLI support now includes `pmp list responses`, `pmp print response <index>`, and `pmp remove response <index>`. Follow-up risk: response CLI actions are index-based, so users need `list responses` first if they are not working from the web UI.
