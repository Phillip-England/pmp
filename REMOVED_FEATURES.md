# Removed Features

## Microphone Input

The browser microphone input flow was removed on May 9, 2026.

Reason:
- It added too much UI and browser-state complexity.
- Long-running speech capture was creating reliability issues.
- The project is now intentionally focused on storing and compiling prompt history.

## Asset Upload and Encryption

The asset upload and encryption system was also removed on May 9, 2026.

Reason:
- It pushed the tool away from its core purpose.
- It introduced extra storage, security, and settings complexity.
