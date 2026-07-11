# VHS tape for clai models demo
# Install: brew install charmbracelet/tap/vhs
# Record: cd .github/demo && vhs demo-models.tape

Output output/demo-models.gif

# Setup
Source demo-setup.tape

# Scene 1: List available models
Type "clai model list"
Sleep 500ms
Enter
Sleep 2s
Hide
Wait@10s
Show
Sleep 3s

# Scene 2: Use a specific model with -m
Ctrl+L
Sleep 500ms
Type "clai -m bedrock/us.amazon.nova-micro-v1:0 summarize manifesto.txt"
Sleep 500ms
Enter
Sleep 2s
Hide
Wait@60s
Show
Sleep 3s
