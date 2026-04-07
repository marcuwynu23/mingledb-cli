# Contributing to mingledb-cli

Thanks for your interest in contributing.

## Getting started

1. Fork the repository and create a feature branch from `main`.
2. Run tests before and after changes:

```bash
go test ./...
```

## Development guidelines

- Keep changes focused and small when possible.
- Add or update tests for behavior changes.
- Keep user-facing CLI commands stable unless a breaking change is intentional and documented.
- Update `README.md` when commands or behavior changes.

## Pull request checklist

Before opening a PR, verify:

- `go test ./...` passes
- New or changed behavior is covered by tests
- Documentation is updated where needed
- Commit messages are clear and describe why the change is needed

## Reporting issues

When opening an issue, include:

- The command you ran and the full output/error
- Expected behavior
- Minimal reproduction steps
- Environment details (Go version, OS)

## Security issues

Please do not open public issues for sensitive vulnerabilities. Instead, contact the maintainers privately.
