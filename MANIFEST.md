# Manifest

Why clai is built the way it is.

---

## clai is a filter

clai takes text, applies an AI transformation, and writes to stdout. By default it holds no state between invocations, requires no interaction, and has no REPL or chat mode.

Conversation persistence is opt-in. When enabled, clai stores message history in flat files and replays it on the next call. Without the flag, every invocation is stateless. The filter model remains the default and the foundation.

The UNIX philosophy made computing powerful by building small, composable tools that combine into systems larger than any single program. `grep`, `sed`, `awk`, `curl`; none of them try to own your workflow. They do one thing well and disappear into your pipeline.

clai brings AI into that pipeline. Not as a platform. As a tool.

---

## POSIX compliance

clai follows POSIX Utility Syntax Guidelines where applicable and extends them with GNU long-option conventions:

- Options precede operands: `clai -m model summarize file.txt`
- `-e` for inline expressions, `-f` for file input (like sed, grep, awk)
- `--` ends option parsing
- Stdin is read automatically when piped (no explicit `-` argument needed)
- Error format: `clai: message` on stderr
- Exit codes: 0 success, 1 runtime error, 2 usage error, 3 schema violation

This means clai works like other UNIX tools. Users don't need to learn clai-specific conventions.

---

## Policy is separated from mechanism

The engine knows how to call a model. It has no opinions about which prompt to use, what model to call, or what output format to enforce.

- Prompt files decide what to instruct the model
- Config decides which provider and model to use
- Flags override both per invocation

The engine is a pure function: `(input, prompt, model, schema) -> output`.

---

## Knowledge lives in prompt files, not in code

The intelligence in clai is in the prompts, not the binary. Adding a new capability means writing a Markdown file, not modifying code. System prompts and strategies ship in `share/clai/` alongside the binary but follow the same format as user-created ones. Users override any system prompt by placing a file with the same name in their config directory.

Schema validation is data-driven (JSON Schema), not hardcoded. Community contributions require no programming knowledge; the barrier is writing a text file.

---

## clai does not acquire content

Fetching content from YouTube, web pages, or any platform is not clai's job. That responsibility belongs in separate tools composed by pipes.

This is a deliberate trade: one-command convenience for beginners is sacrificed for architectural purity, transparency, and user control over intermediate artifacts. The user sees exactly what tools run, can debug each step, and keeps the transcript/video/audio if they want it.

```sh
# YouTube → transcript → summary (user controls each step)
yt-dlp --write-auto-subs --skip-download -o "%(id)s" "URL"
cat *.vtt | sed '/^[0-9]/d; /-->/d' | clai summarize

# Web page → summary
curl -s "https://r.jina.ai/URL" | clai summarize
```

---

## Flags only for what the shell can't do

The shell already handles writing to files (`> file`), appending (`>>`), clipboard (`| pbcopy`), and looping. clai has no `-o`, `--copy`, or `--append` flags.

If a trivial shell construct already does it, don't duplicate it as a flag. Doing so teaches users a clai-specific convention when a universal one already exists.

---

## No output on success except the result

Success produces output on stdout and nothing else. No spinners, no "Done!", no banners. Error messages go to stderr, are one line, and name the problem.

Output that mixes result data with status messages breaks pipelines. A tool that stays silent when things work lets downstream tools trust what they receive.

---

## Failures produce no output

When something goes wrong, clai exits with a non-zero code and writes nothing to stdout. Partial output is worse than no output in a pipeline; it silently corrupts whatever comes next.

---

## Autodetected behavior has no flags

Color output and streaming are detected from the environment using `isatty(stdout)` and the `NO_COLOR` convention. If the program can determine the right behavior from context without asking, it should.

---

## Model behavior flags are allowed; optimization switches are not

`-t` (temperature) and `--think` change *what you get back*. They are allowed. `--top-p` and `--frequency-penalty` tune internal sampling with no user-observable effect that temperature doesn't cover. They are not.

The line: if it changes the character of the response, it's a flag. If it tunes the model's internals, it's not.

---

## Prompts are the unit of community contribution

Prompt repos are plain git repos containing Markdown files. No manifest, no registry, no build step. Anyone can publish by hosting a git repo.

Resolution order: project-local > user-local > system. A team overrides any prompt by placing a file in their repo.

---

## Strategies are separate from prompts

A prompt says *what* to do. A strategy says *how to think*. These are orthogonal.

Baking strategies into prompts hides what's happening. Separating them means:

- Users see and control the reasoning technique
- The same strategy applies to many prompts
- Research updates don't require touching every prompt
- Prompt authors recommend; users override

---

## The command surface stays small

A handful of top-level commands: the filter itself, `prompts`, `strategies`, `models`, and `conversations`. Each manages one concept. A tool that fits within cognitive limits doesn't require a manual for daily use.

---

## No setup command

Configuration is documented; users write config files or set environment variables. The tool requires a model and fails fast with a clear message if none is set.

clai has neither OAuth flows nor interdependent values that justify a wizard. API keys come from env vars. Preferences go in a config file.

---

*When a proposed change conflicts with a decision here, update this document in the same PR. The reasoning matters as much as the decision.*
