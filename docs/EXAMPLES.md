# Examples

Practical examples for using clai in common workflows. For basic usage, see the [README](../README.md).

## clai is built for the UNIX pipeline

`clai` reads from `stdin` and writes results to `stdout`, the same contract as `grep`, `jq`, `awk`, or `sed`. Everything below builds on that:

- **Chain LLM calls.** The output of one `clai` is valid input to the next. `git diff | clai code-review | clai -e "Only critical bugs, numbered"` runs two models back to back, each working on the previous stage's output.
- **Steer any step inline.** `-e "..."` supplies an ad-hoc prompt, so you are never limited to the built-in prompts. Mix named prompts and inline prompts freely in the same pipeline.
- **Emit structured JSON.** `-s '{"field": "type"}'` validates output against a schema and exits non-zero on mismatch, so `clai parse ... | jq` and scripted retries are reliable.
- **Run models locally for privacy.** Point `-m ollama/<model>` at a local endpoint and no data leaves your machine; switch to a frontier model only for the hard steps.
- **Think harder on demand.** `--think` enables extended reasoning on supported providers for hard problems.

The rest of this page is copy-pasteable recipes built on those primitives.

## Content Sources

`clai` takes text and returns results. Fetching content from YouTube, web pages, or other platforms is out of scope and belongs to other tools in your pipeline.

This design trades one-command beginner convenience for:

- **Transparency.** You see exactly what tools run and can debug each step.
- **Artifact control.** Save the transcript, keep the video, reuse intermediate files.
- **Composability.** Mix and match tools; clai doesn't lock you in.

> The examples below show how to use `clai` with third-party tools. Respect copyright, terms of service, and DRM restrictions. You're responsible for how you use them.

### YouTube Transcripts

```sh
# Using yt-dlp (auto-generated subtitles)
yt-dlp --write-auto-subs --skip-download -o "%(id)s" "URL"
cat *.vtt | sed '/^[0-9]/d; /-->/d' | clai summarize

# Using whisper (higher quality)
yt-dlp -x --audio-format wav -o audio.wav "URL"
whisper audio.wav --output_format txt
clai summarize < audio.txt
```

### Web Pages

```sh
# Using Jina Reader
curl -s "https://r.jina.ai/URL" | clai summarize

# Using lynx
lynx -dump "URL" | clai summarize
```

## Pipelines

```sh
# Chain prompts
clai summarize article.txt | clai tweet
git diff HEAD~1 | clai code-review | clai -e "Prioritize by severity"

# Commit message from staged changes
git diff --cached | clai commit

# Code review a PR
gh pr diff 42 | clai code-review

# Explain the last command's error
!! 2>&1 | clai -e "Explain this error and suggest a fix"
```

## Shell Integration

```sh
# Process every file in a directory
for f in docs/*.md; do
  clai -e "Add frontmatter metadata" "$f" > "$f.new" && mv "$f.new" "$f"
done

# Parallel processing with xargs
find . -name "*.go" | xargs -P4 -I{} sh -c 'clai code-review "{}" > "{}.review"'

# Clipboard (macOS)
pbpaste | clai summarize
pbpaste | clai -e "Fix grammar" | pbcopy
pbpaste | clai -e "Convert to bullet points" | pbcopy

# Clipboard (Linux)
xclip -o | clai -e "Fix grammar" | xclip -selection clipboard
```

## Structured Output

```sh
# Parse invoices to JSON (clai reads text, so extract PDF text first)
pdftotext invoice.pdf - | clai parse -s '{"vendor": "str", "amount": "float", "date": "date", "items": "list"}'

# Extract and pipe to jq
clai parse -s '{"title": "str", "tags": "list"}' article.txt | jq '.tags[]'

# Convert prose to CSV
clai to-csv report.txt > data.csv
```

## Git Workflows

```sh
# Review all changes since last release
git log --oneline v0.1.0..HEAD | clai -e "Group these commits by category and summarize"

# Generate changelog entry
git log --oneline v0.1.0..HEAD | clai changelog

# Explain a confusing diff
git show abc1234 | clai -e "Explain what this change does and why"

# Summarize a PR before reviewing
gh pr diff 42 | clai -e "TL;DR of this PR in 3 bullet points"
```

## Model Selection

```sh
# Local model for private content
clai summarize -m ollama/llama3.3 confidential.txt

# Frontier model for hard problems
clai -e "Evaluate this design" --strategy tot -m anthropic/claude-sonnet architecture.md

# Per-prompt model defaults in your shell profile
export CLAI_MODEL_CODE_REVIEW="anthropic/claude-sonnet"
export CLAI_MODEL_SUMMARIZE="openai/gpt-4.1-mini"
export CLAI_MODEL_COMMIT="ollama/llama3.3"
```

## Strategies

```sh
# Debug with step-by-step reasoning
cat failing-test.go | clai -e "Why does this test fail?" --strategy cot

# Explore multiple solutions
clai -e "How should I structure this API?" --strategy tot design.md

# Polish writing with self-critique
clai draft notes.txt --strategy self-refine

# Save tokens in batch jobs
for f in src/*.go; do
  clai code-review --strategy cod "$f" >> reviews.txt
done
```

## Project Configuration

```toml
# .clai/config.toml, committed to repo
model = "openai/gpt-4.1"
temperature = 0.7
```

`.clai/prompts/review.md`, a team-specific code review committed to the repo:

```markdown
---
description: Team code review with our conventions
strategy: cot
---

Review this code following our team conventions:

- We use early returns
- Error handling before happy path
- No nested conditionals deeper than 2 levels
```

## Personal Voice

```sh
# Create your voice file
mkdir -p ~/.config/clai/prompts/shared
cat > ~/.config/clai/prompts/shared/voice.md << 'EOF'
I write in first person. Short sentences. No corporate speak.
Words I never use: leverage, synergy, unlock.
EOF

# Create a prompt that extends a system prompt with your voice
cat > ~/.config/clai/prompts/my-tweet.md << 'EOF'
---
extends: tweet
prepend:
  - shared/voice.md
---
EOF

# Use it
clai my-tweet input.txt
```

## Tips

- **`--dry-run` is free.** Inspect resolved config and assembled prompts before spending tokens.
- **Temperature 0 for extraction.** Use `-t 0` for deterministic structured output.
- **Small models for simple tasks.** A local model handles summarize and translate. Save frontier models for complex analysis.
- **Exit code 3 means schema mismatch.** Retry with a more capable model or simplify the schema.
- **Prompts are files.** Edit them: `$EDITOR $(clai prompt path summarize)`
- **Everything composes.** If clai doesn't do something, pipe to a tool that does.
