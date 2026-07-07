# VHS tape for clai structured output demo
# Install: brew install charmbracelet/tap/vhs
# Record: cd .github/demo && vhs demo-structured.tape

Output output/demo-structured.gif

# Setup
Source demo-setup.tape

# Scene 1: Extract structured data and pipe to jq for colored output
Type 'clai parse -s '
Type "'"
Type '{"author":"str","year":"int"}'
Type "'"
Type ' manifesto.txt | jq'
Sleep 500ms
Enter
Sleep 2s
Hide
Wait@60s
Show
Sleep 4s
