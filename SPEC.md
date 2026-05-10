# pmp

`pmp` is a software written in golang intended to help one keep tabs on their prompt history overtime. For example, you may make multiple prompts to a project and as time goes on, you have no idea how you may have gotten to a certain point. If every prompt is tracked, you can recreate programs on the fly and test different flavors. Hence, `pmp`.

# go mod

This is at github.com/phillip-england/pmp

# A Lack of Heiarchy

Most systems make a huge emphasis on heiarchy, whereas `pmp` makes an emphasis on chronological order. It is more important that w know the order in which prompts were given to the system than it is how things are associated.

# Prompts are Just Markdown

When a prompt is provided to the `pmp` system, it is just stored as markdown with frontmatter to store any metadata like the timestamp or the title.

# Titles are Required

Titles in this system are required. Each prompt at a minimum must have a timestamp and a title.

# vi as the Primary Text Editor

The `pmp` system will attempt to use `vi` to accept user input. If it is not able to, it will instead use your default text editor.

# Instructions Document

PMP ships with built-in compile instructions. They are not user-editable project documentation. They tell a language model what the compiled material is, how to use the instructions, skills, and prompts sections, and that it must write at least one structured response note into `.pmp/responses/` after important work completes.

# User Experience

Here is how the user interacts with the system. They run `pmp init` to initialize a project. That creates `.pmp/` storage. Then they can run `pmp` to open `vi` and write a new prompt. Once they have a `#` header and some body text, they can save the file. `pmp` will scan to ensure a line which begins with `#` and a name following exists, then it will also check for body text. If both of those things are true, it will save the prompt. If not, it will flag the user with an error and next time they open up `pmp` in that directory they will still see their text written there so they do not have to retype.

The user may want to see past entries, so they can run `pmp serve` to open a browser UI. That UI includes pages for prompts, instructions, skills, responses, projects, settings, and an integrated terminal. Terminal sessions can be opened in tabs and should stay alive while the `pmp serve` process keeps running, even when the user navigates elsewhere in the application. Opening a new terminal tab should be quick, including `Cmd+T` on macOS and `Ctrl+T` on Linux and Windows.
