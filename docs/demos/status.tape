# docs/demos/status.tape — render with `vhs status.tape` from this directory.
# Requires bin/spur built (`make build`) and a valid SPUR_TOKEN in the environment.
Output status.gif

Require bash

Set Shell "bash"
Set FontSize 18
Set Width 1200
Set Height 320
Set Padding 20
Set Theme "Catppuccin Mocha"
Set TypingSpeed 50ms
Set Framerate 24
Set WindowBar Colorful
Set BorderRadius 10
Set CursorBlink false

Hide
Type `export PATH="$(git rev-parse --show-toplevel)/bin:$PATH"`
Enter
Type "clear"
Enter
Show

Type "spur status"
Sleep 500ms
Enter
Sleep 4.5s
