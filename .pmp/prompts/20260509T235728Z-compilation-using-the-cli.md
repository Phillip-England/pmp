---
title: "Compilation using the CLI"
timestamp: 2026-05-09T23:57:28Z
---

# Compilation using the CLI

okay lets imagine if we would like to compile using the CLI, well here is how we might do it. We could say: 'pmp compile --from-mark' which would compile from the mark. We could do 'pmp compile --range 0 3' to compile 0 - 3 inclusive. Then we could add this flag to any compilation to ignore updating the mark '--update-mark=false' and then maybe if we want to include skills we could do sometihg like like `pmp compile --from-mark --update-mark=false --skills={'ui-guidelines', 'another-skill'}` so that allows us to successfully do everything we would need to do with the cli for compilation anthing I missed please try to fit that in.
