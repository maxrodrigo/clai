# VHS tape for clai prompts demo
# Install: brew install charmbracelet/tap/vhs
# Record: cd .github/demo && vhs demo-prompts.tape

Output output/demo-prompts.gif

# Setup
Source demo-setup.tape

# Scene 1: Discover available prompts
Type "clai prompt list"
Sleep 500ms
Enter
Sleep 2s
Hide
Wait@60s
Show
Sleep 3s

# Scene 2: Inspect a prompt
Ctrl+L
Sleep 500ms
Type "clai prompt show commit"
Sleep 500ms
Enter
Sleep 2s
Hide
Wait@60s
Show
Sleep 3s

# Scene 3: Use commit prompt on a real diff
Ctrl+L
Sleep 500ms
Type "cat feature.diff | clai commit"
Sleep 500ms
Enter
Sleep 2s
Hide
Wait@60s
Show
Sleep 3s

# Scene 4: Natural language to shell command
Ctrl+L
Sleep 500ms
Type "echo 'list all go files modified in the last 7 days' | clai shell-cmd"
Sleep 500ms
Enter
Sleep 2s
Hide
Wait@60s
Show
Sleep 3s
