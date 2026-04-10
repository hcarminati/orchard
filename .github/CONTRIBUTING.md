# Contributing to Orchard

## Dev environment

```sh
git clone https://github.com/hcarminati/orchard.git
cd orchard
go run ./cmd/orchard     # run locally
go test ./...            # run all tests
go vet ./...             # vet
```

Go 1.22+ is required. No external tools or runtime dependencies needed — the binary is self-contained.

## Branch workflow

1. Fork the repo on GitHub
2. Create a feature branch off `dev`:
   ```sh
   git checkout dev
   git checkout -b feat/your-feature   # or fix/your-fix
   ```
3. Commit your changes and open a PR targeting `dev`
4. `main` is tagged releases only — do not PR directly to `main`

## Code style

- Run `go fmt ./...` and `go vet ./...` before submitting
- Follow the conventions in [CLAUDE.md](../CLAUDE.md): no `interface{}`, no CGO, no runtime dependencies
- Tests are required — a change without tests will not be merged

## Opening a good issue

Please include:

- **Steps to reproduce** — what you ran, what you expected, what happened
- **OS and terminal** — e.g., macOS 15.3, iTerm2 3.5
- **Go version** — `go version`
- **Orchard version** — `orchard --version`

Bug reports without reproduction steps are hard to act on. The more detail you provide, the faster it gets fixed.

## License

By contributing, you agree that your changes are licensed under the project's [MIT License](../LICENSE).
