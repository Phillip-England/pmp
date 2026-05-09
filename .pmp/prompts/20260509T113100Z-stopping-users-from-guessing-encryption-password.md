---
title: "Stopping Users From Guessing Encryption Password"
timestamp: 2026-05-09T11:31:00Z
---

# Stopping Users From Guessing Encryption Password

Users may be tempted to guess an encryption password to unlock assets over and over if they cannot remember their password. Since the password is needed to unlock assets, a form exists to enter in your password to unlock them. Well, this form (and its associated endpoint) should not just allow massive amounts of password guessing. It should only allow 5 guesses per 15 minutes. If you guess more than 5 times in that 15 minute window, you will be locked out from the assets and from guess for that time frame. All of this should be clearly communicated to the user and they should never be in a situation where they accidentally get locked out due to guessing incorrrectly. They should know getting locked out is on the way.
