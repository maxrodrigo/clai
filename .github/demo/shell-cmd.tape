# VHS tape for clai shell-cmd demo
# Install: brew install charmbracelet/tap/vhs
# Record: cd .github/demo && vhs shell-cmd.tape

Output dist/shell-cmd.gif

# Setup
Source setup.tape

# Natural language -> shell command
Type "echo 'find all Go files modified this week over 10KB' | clai shell-cmd"
Sleep 500ms
Enter
Sleep 1s
Hide
Wait@30s
Show
Sleep 3s
