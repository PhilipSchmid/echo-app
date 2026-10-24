# Development

[Back to README](../README.md) · [Configuration](configuration.md)

## Requirements

- Go 1.26 or newer, with a current security patch.
- Make and Bash for the repository targets.
- golangci-lint v2; `make tools` installs the CI version, v2.13.2.
- Docker with Buildx for container builds.
- curl and jq for `scripts/test-image.sh`.

`make tools` also installs the Go protobuf plugins. Install `protoc` separately
only when changing `proto/echo.proto`.

## Build and run

```bash
make deps          # Download dependencies and verify their checksums
make build-quick   # Build ./echo-app without checks
make run           # HTTP and admin listeners
```

Rebuild after changing Go source: the `make run*` targets start the
existing binary.
Use `make run-all` for HTTP, HTTPS, TCP, gRPC, and HTTP/3, or run the binary with
the [configuration flags](configuration.md).
With `entr` installed, `make dev` restarts that binary when Go files change;
it does not rebuild it.

`make build` runs lint, vet, and coverage tests before building. For tests with
the race detector, use `make test` or `make check`.

## Checks

```bash
go mod tidy
git diff --exit-code -- go.mod go.sum
make check
make test-integration
```

`make check` runs lint with errcheck, vet, and race tests. Integration tests build
a temporary application binary and exercise HTTP/1.1 and HTTP/2.0 over plain
HTTP (h2c for HTTP/2.0) and HTTPS, plus HTTP/3, TCP, and gRPC connections.
They also cover response controls, WebSocket echo, readiness transitions, and
shutdown. No curl HTTP/3 support or grpcurl installation is needed for this suite.
Results are saved in
`build/test-results.log`.

Use Go 1.26 with CI's pinned golangci-lint v2.13.2; that linter cannot read
Go 1.27 compiler export data. Local Go 1.27 lint checks pass with v2.15.0.

Useful focused commands:

| Command | Purpose |
| --- | --- |
| `go test -race ./internal/server -run TestHTTPServer` | Run HTTP server tests. |
| `make test-short` | Run tests with Go's short-test flag. |
| `mkdir -p build && make test-coverage` | Write `build/coverage.html`. |
| `make benchmark` | Run Go benchmarks with allocation statistics. |
| `make fmt` | Format Go files. |
| `make lint-fix` | Apply supported linter fixes. |
| `make proto` | Regenerate protobuf sources after schema edits. |

CI also runs staticcheck and the vulnerability scanner:

```bash
GOTOOLCHAIN=go1.26.10 go run golang.org/x/vuln/cmd/govulncheck@v1.3.0 ./...
```

The example selects a patched Go 1.26 toolchain; use newer 1.26 security patches
as they become available. The scanner's v1.3.0 does not support analysis of Go
1.27 source features. CI checks module tidiness and formatting, runs race and
wire tests, and builds five platform targets.

## Cross-platform binaries

```bash
make build-all
```

Binaries are written to `build/` for Linux amd64/arm64, macOS amd64/arm64, and
Windows amd64. Override `BUILD_DIR` to choose another output directory.

To install the executable into Go's binary directory:

```bash
go install ./cmd/echo-app
```

## Containers

```bash
make docker
bash scripts/test-image.sh ghcr.io/philipschmid/echo-app:latest
```

The smoke test starts a non-root container with a read-only filesystem and all
capabilities dropped, checks HTTP/admin endpoints, and verifies clean shutdown.

`make docker-multi` builds Linux amd64/arm64 images with Buildx without publishing
them; output availability depends on the selected Buildx driver.
`make docker-push` publishes the locally available version and `latest` tags;
run it only when publication is intended.

The image workflow publishes branch/SHA tags after checks pass. The release
workflow owns version tags and `latest`; both test an image before publication.

Run `make help` for the complete target list. `make clean` removes local binaries,
the build directory, and coverage files, and clears the Go test cache.
