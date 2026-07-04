# docs/demos/context.tape — render with `vhs context.tape` from this directory.
# Requires bin/spur built (`make build`) and a valid SPUR_TOKEN in the environment.
# The IP is a known Tor exit node so the report shows tunnels and risks.
Output context.gif

Require bash

Set Shell "bash"
Set FontSize 18
Set Width 1200
Set Height 720
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

Type "spur context 185.220.101.34"
Sleep 500ms
Enter
Sleep 6s
