# Contributing

## Issues and security

- **General bugs and features:** open a [GitHub issue](https://github.com/leshchenko1979/ai-gateway/issues) with what you expected, what happened, and how to reproduce when relevant.
- **Security vulnerabilities:** do **not** file a public issue. Follow [SECURITY.md](SECURITY.md).

## Development setup

1. Install **Go 1.25+** (match [`app/go.mod`](app/go.mod)).
2. Copy [`config.yaml.example`](config.yaml.example) to `config.yaml` and set the environment variables referenced in the file (see [README.md](README.md)).
3. Run tests from the `app/` directory (same as CI):

   ```bash
   cd app && go test ./... -count=1
   ```

4. Build the binary from the repository root:

   ```bash
   ./ops.sh build
   ```

5. Format Go code before submitting:

   ```bash
   cd app && gofmt -w .
   ```

## Pull requests

- Branch from `main`, keep commits focused, and reference an issue when one exists.
- Run `go test ./... -count=1` in `app/` and ensure tests pass.
- Match existing style: `gofmt` formatting, package layout, and logging patterns in touched files.
- Update user-facing docs ([README.md](README.md)) if your change affects behavior, configuration, or deployment.
