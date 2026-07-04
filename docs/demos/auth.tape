# docs/demos/auth.tape — render with `vhs auth.tape` from this directory.
# Requires bin/spur built (`make build`) and a valid SPUR_TOKEN in the
# environment so the closing `spur status` succeeds. The token typed into the
# hidden prompt is a placeholder; remove the demo keychain entry afterwards
# with: security delete-generic-password -s spur -a api-token
Output auth.gif

Require bash
Require vhs

Set Shell "bash"
Set FontSize 18
Set Width 1200
Set Height 420
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
Type `security delete-generic-password -s spur -a api-token >/dev/null 2>&1; clear`
Enter
Show

Type "spur auth"
Sleep 500ms
Enter
Sleep 1.5s
Type "sk-spur-demo-1a2b3c4d5e6f7g8h"
Sleep 800ms
Enter
Sleep 2s
Type "spur status"
Sleep 500ms
Enter
Sleep 4.5s
