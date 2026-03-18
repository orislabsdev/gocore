# Contributing to gocore

First off, thank you for considering contributing to `gocore`! It's people like you that make `gocore` a great tool.

## Code of Conduct

By participating in this project, you agree to abide by our Code of Conduct (standard Contributor Covenant).

## How Can I Contribute?

### Reporting Bugs
- Use a clear and descriptive title.
- Describe the exact steps which reproduce the problem.
- Explain which behavior you expected to see instead and why.

### Suggesting Enhancements
- Explain why this enhancement would be useful to most `gocore` users.
- Provide examples of how the new feature would be used.

### Pull Requests
1. Fork the repo and create your branch from `main`.
2. If you've added code that should be tested, add tests.
3. If you've changed APIs, update the documentation.
4. Ensure the test suite passes (`make test`).
5. Make sure your code lints (`make lint` or `golangci-lint run`).

## Style Guide

- We follow the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md).
- Use `gofmt` or `goimports` to format your code.
- Write descriptive commit messages following [Conventional Commits](https://www.conventionalcommits.org/).

## Development Process

- **Branching**: Use feature branches (`feat/`, `fix/`, `docs/`).
- **Testing**: All packages should aim for >80% coverage.
- **Versioning**: We use [Semantic Versioning](https://semver.org/).
