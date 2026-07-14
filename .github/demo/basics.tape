# VHS tape for clai basics demo
# Install: brew install charmbracelet/tap/vhs
# Record: cd .github/demo && vhs basics.tape

Output dist/basics.gif

# Setup
Source setup.tape

# Scene 1: tldr
Type "cat phrack7-3.txt | clai tldr"
Sleep 500ms
Enter
Sleep 1s
Hide
Wait@30s
Show
Sleep 2s

# Scene 2: Explain code — pipe through glow for formatted output
Ctrl+L
Sleep 500ms
Type "clai code-review apple-goto-fail.c | glow"
Sleep 500ms
Enter
Sleep 1s
Hide
Wait@30s
Show
Sleep 3s

# Scene 3: Chaining with web content — fetch an essay and summarize it
Ctrl+L
Sleep 500ms
Type "curl -s 'https://r.jina.ai/https://paulgraham.com/startupideas.html' | clai summarize"
Sleep 500ms
Enter
Sleep 1s
Hide
Wait@30s
Show
Sleep 2s
