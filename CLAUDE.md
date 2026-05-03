# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`bucketfill` is a Go package + CLI for managing **seeds and migrations against bucket/object storage** (the GCS/S3 analogue of `golang-migrate` for SQL). Migrations are versioned Go functions that mutate a bucket's contents; applied state is persisted as a JSON object inside the bucket itself.

Module path: `github.com/n1kola-petrovic/bucketfill` · Go 1.25.5.

## Common commands

```bash
go build ./...              # build all packages
go run . <subcommand>       # run the CLI (subcommands: up, down, version)
go test ./...               # run tests (no test files exist yet)
go vet ./...
go mod tidy
```

There is no Makefile, lint config, or CI yet.

## Architecture

The codebase is split into four layers. Understanding how they connect requires reading across packages:

### `core/` — engine (provider-agnostic)
- **`ObjectStorage` (storage.go)** — the single interface every provider implements: `Upload / Download / Delete / Copy / List`. Anything provider-specific lives behind it.
- **`Client` (client.go)** — user-facing facade migrations actually call. Wraps an `ObjectStorage` + bucket name and exposes ergonomic ops (`Put` auto-detects content type, has a special-case for SVG misdetected as `text/plain`; `Rename` is copy-then-delete; `PutReader` for streaming).
- **`Migration` + global registry (migration.go)** — migrations are `{Version int, Up, Down MigrationFunc}` registered via `core.Register(...)` (typically from a user file's `init()`). The registry is a process-global protected by `sync.Mutex`; `Register` panics on duplicate versions. `ResetMigrations()` exists for tests.
- **`Migrator` (migrator.go)** — applies migrations. `Up` runs every registered migration with `Version > state.Version` in ascending order, **persisting state after each step** (so a mid-run failure leaves a consistent partial state). `Down` rolls back exactly one — the migration matching `state.Version` — and recomputes `state.Version` to the previous registered version (or 0). `Status` prints applied/pending.
- **State (state.go)** — stored *inside the target bucket* as `_bucketfill_state.json` (`{version, appliedAt}`). A missing state object is treated as version 0, not an error. **The state file lives next to user data**; provider `List` calls will surface it.

### `provider/<name>/` — `ObjectStorage` implementations
- `provider/fs` is the only one today. Bucket = root directory; key = path under it (slashes normalized via `filepath.FromSlash`). `Delete` is idempotent (ignores `IsNotExist`); `List` walks the tree and returns slash-separated keys relative to the bucket root.
- New providers (GCS, S3/MinIO, etc.) go under `provider/` as their own packages and only need to satisfy `core.ObjectStorage`.

### `cmd/` — Cobra CLI (`bucketfill up | down | version`)
Wired in each file's `init()` via `rootCmd.AddCommand`. **The `Run` functions are currently empty stubs** — the CLI does not yet construct a `Client`/`Migrator` or load user migrations. Wiring this is open work.

### `migrator/` — empty stubs (`MigrateUp(path string)`, `MigrateDown()`)
Placeholders, presumably the intended seam between `cmd/` and `core/`. Not used yet.

### Mental model of a user workflow
A consumer of this package writes Go files that import `core`, define migrations, and call `core.Register` from `init()`. They build a binary that wires a provider (e.g. `fs.New()`) into `core.NewClient(storage, bucket)` → `core.NewMigrator(client)` → `Up/Down/Status`. The CLI in this repo is intended to be that binary's entry point but isn't fully wired.

## Conventions worth preserving

- All errors from `core` and `provider/*` are wrapped with a package prefix (`bucketfill: …`, `fs: …`) — keep this when adding code.
- `Migrator.Up` writes state after **each** successful migration, not at the end. Don't refactor to a single end-of-run write — partial-progress durability is the point.
- Keys are always slash-separated at the API boundary; providers translate to native separators internally.
- The state key constant `_bucketfill_state.json` is intentionally underscore-prefixed to sort/group separately from user data; don't rename without considering existing buckets.
