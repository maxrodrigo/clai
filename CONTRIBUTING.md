# Contributing to clai

## Development Setup

```sh
git clone https://github.com/maxrodrigo/clai.git
cd clai

make check       # lint + test (CI runs this)
make run ARGS="" # run without installing
```

### Prerequisites

- Go (see `go.mod` for minimum version)
- golangci-lint (`brew install golangci-lint`)

### Available Commands

```sh
make build       # build binary
make run ARGS="" # run during development
make test        # run tests
make lint        # run linter
make check       # lint + test
make tidy        # tidy go.mod
make install     # install to ~/.local (or PREFIX=...)
make uninstall   # remove installed files
make clean       # remove built binary
```

---

## Project Structure

```
cmd/clai/              entry point (main.go only)
internal/
  commands/           cobra command tree
  config/             viper-based config, precedence logic
  datadir/            system data directory resolution
  output/             terminal output, TTY detection
  prompt/             prompt loading, resolution, frontmatter
  provider/           AI provider interface + implementations
  run/                main execution pipeline
  schema/             JSON Schema parsing, validation
  source/             input resolution (files, stdin)
  strategy/           reasoning strategy loading
  version/            build-time version
share/clai/
  prompts/            system prompts
  strategies/         system strategies
```

`internal/` is enforced by the Go compiler — external code cannot import it.

---

## Architecture

### Source Model

Input comes from stdin (read when not a TTY) or file arguments (`os.ReadFile`). Multiple files resolve concurrently and join in declaration order. clai does not fetch URLs — content acquisition belongs in separate tools composed by pipes.

### Config Resolution

From most specific to most general:

1. CLI flag — this invocation
2. Environment variable — this shell session
3. Project config (`.clai/config.toml`) — this project
4. User config (`~/.config/clai/config.toml`) — this user
5. Prompt frontmatter — this prompt's defaults
6. Default

`--dry-run` shows the fully resolved configuration.

### Provider Interface

Implement `provider.Provider`, register via `init()` in `internal/provider/<name>/`, blank-import in `cmd/clai/main.go`.

---

## Code Contributions

1. Fork the repo
2. Create a branch (`git checkout -b fix-something`)
3. Make your changes
4. Run `make check`
5. Submit a PR

### Guidelines

- **Scope:** clai is a text filter. It does not fetch URLs, manage platforms, or hold sessions.
- **Flags:** New flags must align with [MANIFEST.md](docs/MANIFEST.md). If it conflicts, update the manifest in the same PR.
- **Exit codes are API.** Don't change their meanings.
- **`--dry-run` must always work.** It's the primary debugging tool.

### Code Style

- `go fmt` before committing
- Minimal dependencies — prefer standard library
- Error messages: stderr, one line, name the problem
- Tests cover behavior, not implementation. No test-only methods in production code.

### Commit Messages

[Conventional Commits](https://www.conventionalcommits.org/) format:

```
feat: add Gemini provider support
fix: handle empty schema in dry-run mode
docs: update strategy examples
test: add edge case for circular extends
chore: bump golangci-lint action
```

---

## Prompt Contributions

The easiest way to contribute. No Go knowledge required.

1. Create `share/clai/prompts/your-prompt.md` with `description` in frontmatter
2. Test locally: `echo "test" | make run ARGS="--dry-run your-prompt"`
3. Test with multiple models (`ollama/llama3.2` and a frontier model)
4. Verify pipe-cleanliness: `echo "test" | make run ARGS="your-prompt" | wc -w`
5. Test the error case: `echo "" | make run ARGS="your-prompt"`
6. Run `make check`
7. Submit a PR

See [ADVANCED.md](docs/ADVANCED.md) for the full prompt authoring principles and evaluation checklist.

---

## Reporting Bugs

Open an issue with:

1. What you ran (command + input)
2. What you expected
3. What happened instead
4. Output of `clai --version`

Use `clai --dry-run <prompt>` to show resolved configuration without calling the model.

---

## Feature Requests

Before requesting a feature, consider:

- **Is this a prompt, not a feature?** Most "features" are better as prompts.
- **Does this belong in the shell?** clai does not duplicate what `>`, `|`, or `xargs` already do.
- **Does this expand clai's scope?** clai is a text filter, not a platform or agent.

If it's still a feature request, open an issue explaining the use case.
