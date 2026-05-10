---
title: Fixed memory title required error
timestamp: 2026-05-09T21:05:00Z
---

# Fixed memory title required error

Removed the erroneous "body is required" check from `serveMemoryAPI` POST handler. The form was failing validation when body was empty, even though body is optional. Title validation is sufficient.
