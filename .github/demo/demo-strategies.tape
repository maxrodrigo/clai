# VHS tape for clai strategies demo
# Install: brew install charmbracelet/tap/vhs
# Record: cd .github/demo && vhs demo-strategies.tape

Output output/demo-strategies.gif

# Setup
Source demo-setup.tape

# Scene 1: Baseline — no strategy
Type "clai -e 'Solve this problem' math-problem.txt"
Sleep 500ms
Enter
Sleep 2s
Hide
Wait@60s
Show
Sleep 3s

# Scene 2: With chain-of-thought — step-by-step reasoning
Ctrl+L
Sleep 500ms
Type "clai -e 'Solve this problem' --strategy cot math-problem.txt"
Sleep 500ms
Enter
Sleep 2s
Hide
Wait@60s
Show
Sleep 4s
