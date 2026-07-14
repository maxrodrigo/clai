# VHS tape for clai hero demo (README top + HN post)
# Install: brew install charmbracelet/tap/vhs
# Record: cd .github/demo && vhs hero.tape

Output dist/hero.gif

# Setup
Source setup.tape

# Beat 1: the hook — familiar, instant value
Type "cat git.diff | clai commit"
Sleep 500ms
Enter
Sleep 1s
Hide
Wait@30s
Show
Sleep 2s

# Beat 2: the reveal — general filter, composable, steerable with -e
Ctrl+L
Sleep 500ms
Type "clai code-review apple-goto-fail.c | clai -e 'Explain the bug'"
Sleep 500ms
Enter
Sleep 1s
Hide
Wait@30s
Show
Sleep 1.5s
