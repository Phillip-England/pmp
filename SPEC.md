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

# User Experience

Here is how the user interacts with the system. They run `pmp init` to initalize a project. That essentially just creates a directory where a prompts will get stored. Then, they can run `pmp` to open up vi and write a new prompt. Once they have a # header and some body text, they can save the file. `pmp` will scan to ensure a line which begins with '#' and a name following exists, then it will also check for body text. If both of those things are true, it will save the prompt. If not, it will flag the user with an error and next time they open up `pmp` in that directory they will still see their text written there so they do not have to retype. The user may want to see past entires, so they can run 'pmp serve' to serve a webpage in the browser. This page will contain all their entries in order but it will just display the date and title, to see the text one must click the title. It is a plain site not fancy at all very minimal. 50 entries per page and a button exists to pop them all open and pop them all close at top please and thank you. Remember, when a user saves their entry after running `pmp`, it is automatically sorted. The whole point of this system is they do not have to name or sort their files. They simply provide a hashtag header at the top and then provide body text. Once they save after that the file will clear and be sorted.
