---
title: "Changing \"Responses\" To \"History\""
timestamp: 2026-05-11T00:13:24Z
---

# Changing "Responses" To "History"

Instead of the llm leaving us responses in the resoonses folder, lets create a "history" folder instead and migrate this feature over. We can modify our project instructions to guide the llm to record project history. Essentially the llm should be told they need to leave behind a markdown note that tells exactly what has change during that interaction. In this way, you end up with two histories in the project, you have a prommpt history, then you have the llms recorded history which together can help create a more full scope of a project down the line.
