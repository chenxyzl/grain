# Repository Guidelines

## Project Structure & Module Organization

The root `grain` package implements actor lifecycle, mailboxes, reentrant requests, scheduling, and cluster routing. `message/` and `remote/` contain protobuf definitions and generated Go code. Utilities live in `al/`, `uuid/`, and `ghelper/`. Tests sit alongside implementation files as `*_test.go`.

`examples/` is a separate Go module linked through `go.work`. Read `README.md`, `README_ZH.md`, and `docs/reentrancy.md` for usage and execution semantics.

## Build, Test, and Development Commands

Use Go 1.27, as required by the root module and workspace.

- `go build ./...`: build the library and root-module packages.
- `go test ./...`: run root-module tests; this excludes the nested examples module.
- `go test -race . -run 'Test(Reentrancy|SelfAsk|Ask|PreStop)'`: check request and lifecycle concurrency.
- `go test . -run '^$' -bench BenchmarkRingBufferPushPop -benchmem`: measure queue performance and allocations.
- `cd examples && go run ./hello_first`: run the introductory example with etcd listening on `127.0.0.1:2379`.
- `make proto`: regenerate message and transport bindings using the protobuf tools configured by the Makefile.

## Coding Style & Naming Conventions

Format changed Go files with `gofmt`; use tabs and standard Go naming. Exported identifiers use PascalCase, internal identifiers use camelCase, and files follow existing names such as `processor_mailbox.go`. Keep invariant comments close to synchronization code. Edit `.proto` sources and regenerate bindings instead of editing generated `*.pb.go` files.

## Testing Guidelines

Use Go's `testing` package with `TestBehavior` and `BenchmarkOperation` names. Add focused regression tests for changed behavior, especially reentry, shutdown, and message delivery. Prefer channels for deterministic coordination. Run race checks for concurrency changes. No fixed coverage threshold is documented; compare benchmark allocations and timing when claiming performance improvements.

## Commit & Pull Request Guidelines

History uses prefixes including `fix:`, `perf:`, `ref:`, and combinations such as `perf+fix:`; Chinese descriptions are common. Keep commits focused. PRs should explain the problem, resulting behavior, and validation commands, link relevant issues, and include measurements for performance claims.

## Actor Execution & Code Navigation

Only call reentrant `BaseActor.Ask` from the current normal handler, never a spawned goroutine or lifecycle hook. Preserve serialized actor execution and shutdown accounting for suspended handlers.

If `.codegraph/` exists at the repository root, use `codegraph_explore` or `codegraph explore "<question or symbol>"` before searching or reading code. Otherwise, skip CodeGraph; do not initialize an index automatically.
