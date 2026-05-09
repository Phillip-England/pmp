---
title: "Keeping Documents Encrypted for Version Control"
timestamp: 2026-05-09T11:20:07Z
---

# Keeping Documents Encrypted for Version Control

Since this application crates assets that are intended to live with the project long-term, and since we will be making references to difference documents and such in these projects, we need a way to encrypt the documents so they can live with our version control. This will require some root-level password to essentially unlock encypted assets after they are pulled down from github. So here is how the workflow will go: we will upload an asset on the "assets" page. Then, we give the asset a name. They pmp system will encrypt the asset using our root-level password. If we do not have a root level password, the pmp system will prompt us to create one. It will inform us very clearly that this password is being used to encrypt assets and that losing the password means losing access to the data. This should allow us to upload assets, encrypt them, then store them in version control. Then, when we pull them down, and make referene to them, we can actually pull those assets into the compiled output by unencrypting them, placing them in the footer of our compiled output as a reference, and than go through all of our @tags and make the links apparent. This converts pmp into a system which allows us to carry assets with our llm prompt history in a powerful encrypted way. This means we cannot double name assets and that they all must be independant.
