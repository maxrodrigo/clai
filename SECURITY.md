# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| latest  | :white_check_mark: |

Once stable releases begin, this table will specify which versions receive security updates.

## Reporting a Vulnerability

**Do not report security vulnerabilities through public GitHub issues.**

Please report them through [GitHub's private vulnerability reporting](https://github.com/maxrodrigo/clai/security/advisories/new).

Include as much of the following as possible:

- Type of vulnerability (e.g., credential exposure, command injection, path traversal)
- Full paths of affected source files
- Step-by-step instructions to reproduce
- Proof-of-concept or exploit code (if available)
- Impact assessment

You should receive an initial response within 48 hours. We will keep you informed of progress and may ask for additional details.

## Security Considerations

### API Keys and Credentials

clai handles API keys for LLM providers. Keep the following in mind:

- **Environment variables** are the recommended method for supplying credentials (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, etc.)
- **Config files** support `${VAR}` syntax to reference environment variables — avoid hardcoding keys
- **Never commit credentials** to version control
- **`--dry-run` is safe** — it never contacts any provider, making it suitable for debugging without credential risk

### Input and Output

- clai processes arbitrary text from stdin and files; that text is sent to third-party LLM providers
- clai writes only to stdout and stderr — it does not create or modify files during normal operation

## Security Best Practices for Users

1. Rotate API keys periodically and immediately after any suspected exposure
2. Use environment variables for credentials; use config files only for non-sensitive settings
3. Review prompts before running them against sensitive data — prompt content is forwarded to third-party providers
4. Prefer `--dry-run` when testing pipelines that handle confidential input
