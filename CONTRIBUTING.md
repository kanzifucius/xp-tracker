# Contributing to xp-tracker

Thanks for your interest in contributing to xp-tracker. This document covers the workflow and conventions for contributing.

## Getting Started

### Prerequisites

- Go 1.25+ (or use [mise](https://mise.jdx.dev/): `mise install`)
- Docker (for container builds)
- kubectl
- [kindplane](https://github.com/kanzi/kindplane) (recommended for local dev)

### Development Setup

```bash
# Clone your fork
git clone https://github.com/<you>/xp-tracker.git
cd xp-tracker

# Pin tool versions (optional, uses .mise.toml)
mise install

# Bootstrap a full local environment (Kind cluster + Crossplane + Prometheus + sample resources)
make dev

# Run the exporter pre-configured for sample resources
make run-local

# See all available Make targets
make help
```

### Running Tests

| Command      | Description                              |
|--------------|------------------------------------------|
| `make test`  | Run tests with race detector             |
| `make lint`  | Run golangci-lint                        |
| `make vet`   | Run go vet                               |
| `make fmt`   | Run gofmt                                |
| `make check` | Run all checks (fmt, vet, lint, test)    |
| `make ci`    | CI-equivalent checks                     |

## Making Changes

### Branching

Fork the repo and create a feature branch from `main`:

- `feature/description` -- new functionality
- `fix/description` -- bug fixes
- `docs/description` -- documentation changes

### Code Style

- Code must pass `gofmt` (run `make fmt`)
- Code must pass `golangci-lint` (run `make lint`)
- Use `log/slog` for logging (JSON handler)
- No CGO -- all builds are static (`CGO_ENABLED=0`)

### Commit Messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
type: description
```

Types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `ci`

- Keep the first line under 72 characters
- Reference issues where applicable: `fix: handle nil pointer in poller (#42)`

### Pull Requests

- Fill out the PR template
- Link related issues
- Ensure CI passes (`make ci`)
- Keep PRs focused -- one feature or fix per PR
- Update documentation if behavior changes

## Reporting Issues

- Use the **bug report** template for bugs
- Use the **feature request** template for enhancements
- Search existing issues before creating a new one

## Code of Conduct

All contributors are expected to follow the [Code of Conduct](CODE_OF_CONDUCT.md).

## License

Contributions are licensed under the [Apache 2.0 License](LICENSE). By contributing, you agree that your contributions will be licensed under the same terms.
