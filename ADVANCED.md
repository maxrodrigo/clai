# Advanced Usage

This guide covers prompt authoring and reasoning strategies in depth. For basic usage, see the [README](README.md).

---

## Prompt Authoring

### Philosophy

A clai prompt is a system instruction delivered to an LLM. The user's input arrives separately as the user message. The model's output goes to stdout and into a pipe.

Three constraints follow from this:

1. **Output is data, not conversation.** No preambles ("Here's your summary:"), no sign-offs. The output must be directly consumable by the next tool in the pipeline.
2. **The prompt must work across model sizes.** Built-in prompts run on small local models as well as frontier models.
3. **The prompt is the entire interface.** No follow-up, no clarification, no retry loop. The prompt must be unambiguous enough to produce useful output on the first call.

### Structure

Every system prompt follows this structure. Use only what the prompt needs.

```
[Role]              — WHO the model is (specific, not flattery)
[Task]              — WHAT to do (one imperative sentence)
[Quality criteria]  — What GOOD output looks like
[Constraints]       — Boundaries, positively framed
[Output format]     — Structure of the response (pipe-friendly)
[Edge cases]        — What to do with degenerate input
[Example]           — One input/output pair when format is non-obvious
```

**Role** — Not decoration. It primes behavior. Specify domain expertise, approach, and values.

- Bad: `You are an expert programmer.`
- Good: `You are a senior developer who prioritizes readability and correctness over cleverness.`

**Task** — One sentence. Imperative mood. States the transformation.

- Bad: `Please help the user by analyzing their code and providing feedback.`
- Good: `Given source code, identify bugs, security issues, and performance problems.`

**Quality criteria** — What separates good output from mediocre. The most underused technique.

- Bad: `Write a good summary.`
- Good: `Each point should be independently meaningful — a reader who sees only that point should understand the claim without needing the others.`

**Constraints** — State what the model SHOULD do, not what it shouldn't. When a negative constraint is necessary, include the reason.

- Bad: `Do not use bullet points.`
- Good: `Output flowing prose, not bullet points — this output feeds into text-to-speech.`

**Output format** — Plain text by default. No wrapping boilerplate. Predictable structure.

**Edge cases** — Handle empty input, wrong-type input, insufficient input. A short, parseable error message — not silence, not hallucination.

**Examples** — One well-chosen example outperforms paragraphs of explanation. Show the shape, not a full real-world case.

### Principles

1. **Every token carries signal.** No "please", no "make sure to", no "I want you to". If it doesn't constrain the output, it doesn't belong.

2. **Specificity over intensity.** `Output valid JSON. If the input cannot be structured, return {"error": "reason"}.` beats `CRITICAL: You MUST output valid JSON.`

3. **Quality criteria beat word counts.** `Each point should be a single concrete claim — no filler, no hedging.` beats `Write each point in exactly 16 words.`

4. **One example > ten rules.** Use `<example>` tags to delimit examples clearly.

5. **The model mirrors your register.** If your prompt is direct and terse, the output will be direct and terse.

6. **Motivation unlocks generalization.** A constraint with a reason generalizes to cases you didn't enumerate.

7. **Handle failure explicitly.** `If the input contains no code, respond with exactly: "error: input is not code"`

### Evaluation Checklist

Before a prompt ships as a system prompt:

- [ ] Works on a small local model with a real-world input
- [ ] Works on a frontier model with the same input
- [ ] Output has no preamble or postamble (pipe-clean)
- [ ] `echo "" | clai <prompt>` produces a sensible error, not hallucination
- [ ] Output composes — piping into `wc`, `grep`, `jq`, or another `clai` call makes sense
- [ ] Prompt is under 200 words (excluding example)
- [ ] Every sentence in the prompt would change the output if removed
- [ ] No superlatives, no filler, no "please"

### What clai Prompts Are NOT

- **Not agent instructions.** Single-turn system messages. No tool use, no memory.
- **Not prompt-engineering patterns.** No "take a deep breath", no arbitrary word counts.
- **Not templates with blanks.** They work as-is with any valid input.
- **Not optimized for one model.** Must degrade gracefully, not catastrophically.

---

## Frontmatter Reference

Every prompt file is Markdown with an optional YAML frontmatter block.

```markdown
---
description: One-line summary shown in `clai prompts`
model: provider/model-name
temperature: 0.7
strategy: cot
schema: '{"field": "type"}'
think: true
prepend:
  - shared/voice.md
append:
  - shared/output-rules.md
---
Your prompt body here.
```

| Field | Type | Effect |
|-------|------|--------|
| `description` | string | Shown in `clai prompts`. Required for system prompts. |
| `model` | string | Default model. Format: `provider/model-name`. Overridden by `-m`, `CLAI_MODEL_<NAME>`, config. |
| `temperature` | float | Sampling temperature (0.0–2.0). Overridden by `-t`. |
| `strategy` | string | Default reasoning strategy. Overridden by `--strategy`. |
| `schema` | string | JSON Schema for structured output. Overridden by `-s`. |
| `think` | bool | Enable extended thinking (Anthropic/Bedrock). Overridden by `--think`. |
| `extends` | string | Inherit body and frontmatter from another named prompt. |
| `prepend` | list | Files prepended before the prompt body. Relative to prompt file's directory. |
| `append` | list | Files appended after the prompt body. Same path resolution. |

### When to Use Each Field

**`model`** — Only when the prompt *requires* capabilities that smaller models lack:

```yaml
model: openai/gpt-4.1  # needs complex JSON output
```

**`temperature`** — When the task has a natural temperature:

```yaml
temperature: 0    # extraction tasks
temperature: 0.9  # creative rewriting
```

**`strategy`** — When the task reliably benefits from structured reasoning:

```yaml
strategy: cot  # code review benefits from step-by-step
```

**`think`** — For genuinely hard problems. Doubles token cost and latency.

**`extends`** — Build on a system prompt without duplicating it:

```yaml
extends: tweet
description: Tweet in my voice
prepend:
  - shared/voice.md
```

---

## Composition with Prepend/Append

Shared concerns live in separate files and get assembled at runtime.

### Voice Consistency

```markdown
---
prepend:
  - shared/voice.md
---
Summarize the following text...
```

Where `shared/voice.md`:
```
Write in active voice. Prefer short sentences. Address the reader directly.
No marketing language, no buzzwords, no hedging.
```

### Recommended Layout

```
~/.config/clai/prompts/
├── shared/
│   ├── voice.md
│   ├── no-preamble.md
│   └── json-rules.md
├── summarize.md          # prepend: [shared/voice.md]
├── code-review.md        # append: [shared/no-preamble.md]
└── parse.md              # append: [shared/json-rules.md]
```

### Recipe: Personal Voice

Create your voice file:

```sh
mkdir -p ~/.config/clai/prompts/shared
cat > ~/.config/clai/prompts/shared/voice.md << 'EOF'
I write in first person. Short sentences. No corporate speak.
I use concrete examples from my own experience.
Words I never use: leverage, synergy, unlock, excited to announce.
EOF
```

Create a wrapper that extends a system prompt:

```sh
cat > ~/.config/clai/prompts/my-tweet.md << 'EOF'
---
description: Tweet in my voice
extends: tweet
prepend:
  - shared/voice.md
---
EOF
```

Use it:

```sh
clai my-tweet input.txt
```

---

## Strategies In Depth

Strategies are research-backed reasoning techniques prepended to the system prompt at runtime. They change *how* the model thinks, not *what* it's asked to do.

### Resolution

Strategies resolve from multiple sources (first match wins):

1. `.clai/strategies/` — Project-local
2. `~/.config/clai/strategies/` — User
3. System (`share/clai/strategies/`)

Precedence: `--strategy` flag > config file > prompt frontmatter.

Use `--strategy none` to disable any strategy.

### Chain of Thought (`cot`)

Think step by step, showing reasoning explicitly.

**Research:** Wei et al., 2022 — [Chain-of-Thought Prompting Elicits Reasoning in Large Language Models](https://arxiv.org/abs/2201.11903)

**Output format:**
```
**Reasoning:** [step-by-step thinking]
**Answer:** [final answer]
```

**When to use:** Multi-step reasoning, debugging, code analysis, problems where the answer depends on intermediate conclusions.

**When to avoid:** Simple lookups, tasks where only the final output matters, token-constrained environments.

### Chain of Draft (`cod`)

Minimal, dense notes before answering. Same rigor as CoT at ~7.6% of the tokens.

**Research:** Xu et al., 2025 — [Chain of Draft: Thinking Faster by Writing Less](https://arxiv.org/abs/2502.18600)

**Output format:**
```
**Draft:** [minimal reasoning notes]
**Answer:** [full final answer]
```

**When to use:** Token-constrained batch processing, straightforward problems that still benefit from explicit steps, production pipelines.

**When to avoid:** Complex reasoning where compression loses nuance, when you need to audit the reasoning.

### Tree of Thought (`tot`)

Explore 2-3 independent approaches, evaluate each, converge on the best.

**Research:** Yao et al., 2023 — [Tree of Thoughts: Deliberate Problem Solving](https://arxiv.org/abs/2305.10601)

**Output format:**
```
**Branch A:** [approach and conclusion]
**Branch B:** [approach and conclusion]
**Best approach:** [which and why]
**Answer:** [final answer]
```

**When to use:** Ambiguous problems, design decisions with trade-offs, debugging unclear root causes, architecture questions.

**When to avoid:** Problems with one clear approach, token-constrained environments (most expensive strategy), time-sensitive tasks.

### Self-Refine (`self-refine`)

Draft, critique, improve. Mirrors write-review-revise.

**Research:** Madaan et al., 2023 — [Self-Refine: Iterative Refinement with Self-Feedback](https://arxiv.org/abs/2303.17651)

**Output format:**
```
**Draft:** [initial answer]
**Critique:** [what is weak or wrong]
**Refined answer:** [improved response]
```

**When to use:** Writing tasks, documentation, code where edge cases are missed on first pass, final outputs humans will read.

**When to avoid:** Right-or-wrong problems (math, logic), token-constrained, outputs that will be further processed.

### Choosing a Strategy

| Situation | Recommended | Why |
|-----------|-------------|-----|
| Debugging a complex issue | `cot` or `tot` | Trace logic or explore hypotheses |
| Batch processing 100 files | `cod` | Reasoning at minimal cost |
| Writing documentation | `self-refine` | Prose always improves with revision |
| Simple code generation | `none` | Overhead adds no value |
| Architectural decisions | `tot` | Explicit trade-off comparison |
| Code review | `cot` | Sequential analysis catches issues |
| Summarization | `cod` or `none` | Minimize overhead |

### Creating Custom Strategies

Create a `.md` file in `~/.config/clai/strategies/`:

```markdown
# Step-Back Prompting

Before solving this problem directly, take a step back and identify the general principle or concept that applies. First articulate the high-level abstraction, then use it to solve the specific problem.

Format your response as:
**Principle:** [the general concept that applies]
**Application:** [how it applies to this specific problem]
**Answer:** [your solution]
```

The first `# ` line becomes the description (shown in `clai strategies`). Everything after is the prompt text prepended to the system prompt.

```sh
clai strategies                # list all
clai strategies show cot       # view a strategy
clai strategies path cot       # file path (for editing)
```

---

## Configuration Reference

Below is a complete configuration file showing all available options and their defaults. Copy what you need into `~/.config/clai/config.toml` (user) or `.clai/config.toml` (project).

```toml
# Model in provider/model-name format.
# No default — must be set via config, env, or flag.
# model = "openai/gpt-4.1"

# Sampling temperature (0.0–2.0).
# Default: unset (uses the provider's default).
# temperature = 1.0

# Maximum tokens to generate.
# Default: 0 (no limit / provider default).
# max_tokens = 4096

# Reasoning strategy applied to the prompt.
# Options: cot, cod, tot, self-refine, none.
# Default: "" (no strategy).
# strategy = ""

# Enable extended thinking (Anthropic/Bedrock only).
# Default: false.
# think = false

# Token budget for extended thinking.
# Default: 0 (provider decides).
# think_budget = 0

# Continue processing remaining sources if one fails.
# Default: false.
# continue_on_error = false

# --- Providers ---
# Each provider needs at minimum a base_url.
# API keys support ${ENV_VAR} expansion.

[providers.openai]
base_url = "https://api.openai.com/v1"
api_key = "${OPENAI_API_KEY}"
# timeout = 0  # request timeout in seconds (0 = no timeout)

[providers.anthropic]
base_url = "https://api.anthropic.com"
api_key = "${ANTHROPIC_API_KEY}"
# timeout = 0

[providers.bedrock]
base_url = "https://bedrock-runtime.us-east-1.amazonaws.com"
api_key = "${AWS_BEARER_TOKEN_BEDROCK}"
# timeout = 0

[providers.ollama]
base_url = "http://localhost:11434/v1"
# api_key = ""  # not required for local Ollama
# timeout = 0

# Custom OpenAI-compatible providers follow the same schema:
# [providers.your-provider]
# base_url = "https://api.example.com/v1"
# api_key = "${YOUR_PROVIDER_KEY}"
# timeout = 0
```

### Environment Variables

All top-level config keys can be set with the `CLAI_` prefix:

| Variable | Config Equivalent |
|----------|-------------------|
| `CLAI_MODEL` | `model` |
| `CLAI_TEMPERATURE` | `temperature` |
| `CLAI_MAX_TOKENS` | `max_tokens` |
| `CLAI_STRATEGY` | `strategy` |
| `CLAI_THINK` | `think` |
| `CLAI_THINK_BUDGET` | `think_budget` |
| `CLAI_CONTINUE_ON_ERROR` | `continue_on_error` |

Per-prompt model overrides use the pattern `CLAI_MODEL_<PROMPT_NAME>` (uppercase, hyphens become underscores):

```sh
export CLAI_MODEL_CODE_REVIEW="anthropic/claude-sonnet"
export CLAI_MODEL_SUMMARIZE="openai/gpt-4.1-mini"
```

Provider API keys use their standard environment variables (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `AWS_BEARER_TOKEN_BEDROCK`).

### Precedence

Configuration is merged in order (later overrides earlier):

1. Built-in defaults
2. Prompt frontmatter defaults
3. User config (`~/.config/clai/config.toml`)
4. Project config (`.clai/config.toml`)
5. Environment variables (`CLAI_*`)
6. CLI flags

---

## Further Reading

- Wei et al., 2022. [Chain-of-Thought Prompting Elicits Reasoning in Large Language Models](https://arxiv.org/abs/2201.11903)
- Xu et al., 2025. [Chain of Draft: Thinking Faster by Writing Less](https://arxiv.org/abs/2502.18600)
- Yao et al., 2023. [Tree of Thoughts: Deliberate Problem Solving with Large Language Models](https://arxiv.org/abs/2305.10601)
- Madaan et al., 2023. [Self-Refine: Iterative Refinement with Self-Feedback](https://arxiv.org/abs/2303.17651)
