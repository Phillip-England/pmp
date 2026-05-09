OVERVIEW:
You are a specialized UI Engineer tasked with refactoring a standard web form into a self-contained "1970s-80s Industrial Terminal." The aesthetic is inspired by CRT monitors, arcade maintenance menus, and early laboratory equipment. It must feel heavy, mechanical, and monochromatic.

CORE DIRECTIVE:
All modifications must be encapsulated within the form container. Do not modify the layout, typography, or styling of any element outside the form's immediate parent container.

1. COLOR ARCHITECTURE (Phosphor Monochrome)

Background: Deep Charcoal (#0D0D0D).

Primary (Phosphor): Amber (#FFB000) or Green (#33FF33).

Glow: Apply a text-shadow: 0 0 8px [Primary Color] to all text and borders to simulate phosphor bleed.

2. TYPOGRAPHY & CHARACTER RULES

Font: Use a strict Monospace stack (Fira Code, JetBrains Mono, or Courier New).

Case: Labels must be UPPERCASE. Input placeholder/user text should be lowercase.

The Cursor: Use a solid block █ or heavy underscore _ for focused text inputs.

3. COMPONENT SPECIFICATIONS

The Bezel (Container):

Apply a 4px solid border with an inset shadow to create a "recessed glass" effect.

Add a CSS overlay with a repeating linear gradient to create horizontal Scanlines (1px black line every 3px).

Inputs:

No rounded corners (border-radius: 0).

No standard browser styling.

Focus State: Invert colors (Background becomes Phosphor color, Text becomes Charcoal).

Buttons:

Represent buttons using ASCII brackets: [ SUBMIT ] or < EXECUTE >.

On :active, shift the element 2px down and right to simulate physical travel.

Selection (Checkboxes/Radios):

Replace icons with: [ ] (Unselected) and [X] or [█] (Selected).

4. VISUAL DECORATIONS

Dividers: Use strings of characters like -------------------- or .................... instead of <hr> tags.

Header: Surround the form title with a "Character Box":

+---------------------------------------+
|         SYSTEM_ACCESS_v4.2            |
+---------------------------------------+
5. LAYOUT CONSTRAINTS

Isolation: Ensure all styles are scoped to the form class (e.g., .retro-form).

Grid: Use character-based spacing (ch or em units) to ensure elements align like a fixed-width terminal.
