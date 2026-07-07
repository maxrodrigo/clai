# VHS tape for clai hero demo
# Install: brew install charmbracelet/tap/vhs
# Record: cd .github/demo && vhs demo-basics.tape

Output output/demo.gif

# Setup
Source demo-setup.tape

# Scene 1: tldr
Type "cat manifesto.txt | clai tldr"
Sleep 500ms
Enter
Sleep 2s
Hide
Wait@60s
Show
Sleep 3s

# Scene 2: Code review — pipe through glow for formatted output
Ctrl+L
Sleep 500ms
Type "clai code-review apple-goto-fail.c | glow"
Sleep 500ms
Enter
Sleep 2s
Hide
Wait@60s
Show
Sleep 1s
ScrollDown 3
Sleep 1s
ScrollDown 3
Sleep 3s

# Scene 3: Explicit prompt — ask anything
Ctrl+L
Sleep 500ms
Type "clai -e 'Extract the key argument in one sentence' manifesto.txt"
Sleep 500ms
Enter
Sleep 2s
Hide
Wait@60s
Show
Sleep 3s

# Scene 4: Model selection — use a different model
Ctrl+L
Sleep 500ms
Type "clai -v summarize -m bedrock/us.amazon.nova-micro-v1:0 manifesto.txt"
Sleep 500ms
Enter
Sleep 2s
Hide
Wait@60s
Show
Sleep 3s

# Scene 5: Chaining with web content — summarize essay and tweet it
Ctrl+L
Sleep 500ms
Type "curl -s 'https://r.jina.ai/https://paulgraham.com/startupideas.html' | clai summarize | clai tweet"
Sleep 500ms
Enter
Sleep 2s
Hide
Wait@120s
Show
Sleep 4s
