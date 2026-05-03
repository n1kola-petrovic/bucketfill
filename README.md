# bucketfill

Versioned seeds and migrations for object storage — like `golang-migrate` for SQL, but for buckets. Works with the local filesystem, Google Cloud Storage, and AWS S3 / S3-compatible services.

Each version lives in `migrations/v<N>/` with a `data/` folder (whose nesting mirrors the bucket layout) plus `up.go` and `down.go` Go files. The scaffolded `up.go` defaults to `c.PutAll(ctx)` (upload the whole `data/` tree) and `down.go` defaults to `c.DeleteAll(ctx)` (remove the same files) — but they're real Go files you can edit when a particular version needs custom logic.

A single `migrations/embed.go` covers every version via one `//go:embed all:v*` glob, and `cmd/migrate/main.go` is regenerated automatically as you add versions, so you never edit it by hand. Apply with `bucketfill up`, roll back with `bucketfill down`. State is tracked inside the bucket itself.

`bucketfill` ships as both a **CLI** and a **Go library** — pick whichever fits.

## Install

The CLI:

```bash
go install github.com/n1kola-petrovic/bucketfill/cmd/bucketfill@latest
```

That drops the `bucketfill` binary at `$(go env GOBIN)` (or `~/go/bin` if `GOBIN` is unset). Make sure that directory is on your `$PATH`.

To pin a release: `...@v0.1.0`. To use the tip of `main`: `...@main`.

The library (added implicitly when the CLI scaffolds a project; you can also add it directly):

```bash
go get github.com/n1kola-petrovic/bucketfill
```

## Quick start (CLI)

From any Go module:

```bash
bucketfill init             # writes bucketfill.yaml
# edit bucketfill.yaml — set provider, bucket, optionally rename migrationDir

bucketfill new              # creates migrations/v1/{up.go, down.go, data/}
                            # also writes migrations/embed.go + cmd/migrate/main.go
# drop your assets into migrations/v1/data/<wherever you want them in the bucket>
bucketfill up               # apply all pending migrations

bucketfill new              # creates migrations/v2/{up.go, down.go, data/}
                            # main.go is regenerated to register v2
# drop files into migrations/v2/data/
bucketfill up               # picks up v2

bucketfill status           # see what's applied vs pending
bucketfill down             # roll back the most recent migration
bucketfill down --to 0      # roll back everything
```

The default `up.go` body is `c.PutAll(ctx)`; the default `down.go` is `c.DeleteAll(ctx)`. Edit either file to customize that version's logic. `migrations/embed.go` (the single `//go:embed all:v*`) is created once and never touched again. `cmd/migrate/main.go` is regenerated on every up/down/status to register whatever versions exist — you never edit it by hand.

`bucketfill init` only writes the yaml. The migrations directory is created the first time you run `bucketfill new`, using whatever `migrationDir` says in your config — so feel free to rename it from `migrations` to `db/seeds`, `bucket-migrations`, or whatever fits your project.

## Project layout

After `bucketfill new` for v1 and v2, a project looks like:

```
my-project/
├── bucketfill.yaml            # provider, bucket, migrationDir
├── go.mod
├── migrations/
│   ├── embed.go               # one file, scaffolded once; //go:embed all:v*
│   ├── v1/
│   │   ├── up.go              # func Up(ctx, *bucketfill.Client) error  — defaults to PutAll
│   │   ├── down.go            # func Down(ctx, *bucketfill.Client) error — defaults to DeleteAll
│   │   └── data/              # nesting mirrors the bucket layout
│   │       └── seeds/welcome.txt
│   └── v2/
│       ├── up.go
│       ├── down.go
│       └── data/...
└── cmd/migrate/
    └── main.go                # auto-regenerated to register every v<N>/
```

**Adding a new migration version:**

```bash
bucketfill new          # creates migrations/v<N+1>/{up.go, down.go, data/}
                        # main.go is regenerated to register v<N+1>
# drop your files into migrations/v<N+1>/data/
bucketfill up
```

You only edit `up.go`/`down.go` if that specific version needs logic beyond the defaults. The single `migrations/embed.go` and the auto-regenerated `cmd/migrate/main.go` mean you never hand-write the version embed or registration boilerplate.

## Writing a custom migration

The scaffolded `up.go` and `down.go` for each version look like:

```go
// migrations/v3/up.go
package v3

import (
    "context"

    "github.com/n1kola-petrovic/bucketfill"
)

func Up(ctx context.Context, c *bucketfill.Client) error {
    return c.PutAll(ctx)   // default: upload everything in v3/data/
}
```

For most migrations that's all you need — leave it alone. When a specific version needs custom behavior (a rename, a conditional, a one-off transform), edit those files:

```go
func Up(ctx context.Context, c *bucketfill.Client) error {
    if err := c.PutAll(ctx); err != nil {
        return err
    }
    return c.Rename(ctx, "old/key.txt", "new/key.txt")
}
```

The Client is scoped to the bucket configured in `bucketfill.yaml`, with `c.Put` / `c.PutAll` / `c.DeleteAll` resolving sources from this version's `data/` subtree (wired in via `fs.Sub` on the top-level embed.FS).

### Client API

| Method | What it does |
|---|---|
| `c.Put(ctx, "seeds/logo.svg")` | Upload `data/seeds/logo.svg` to bucket key `seeds/logo.svg` (same path on both sides). |
| `c.PutAll(ctx)` | Recursively mirror the entire `data/` tree into the bucket. |
| `c.PutFromPath(ctx, "key", "/abs/or/rel/path")` | Upload a file from anywhere on disk (escape hatch for files outside `data/`). |
| `c.PutReader(ctx, "key", r, size, contentType)` | Stream bytes from an `io.Reader`. |
| `c.Get(ctx, "key")` | Read an object back (returns `io.ReadCloser`). |
| `c.Delete(ctx, "key")` | Delete an object (idempotent). |
| `c.Rename(ctx, "old", "new")` | Move an object (copy + delete). |
| `c.List(ctx, "prefix")` | List keys under a prefix. |

Content type is auto-detected on `Put` / `PutFromPath` (with a fix for SVG, which Go's stdlib mis-detects as `text/plain`).

## Configuration

`bucketfill.yaml` (created by `bucketfill init`):

```yaml
provider: fs                # fs | gcs | s3
bucket: default
migrationDir: migrations    # rename to anything you like

fs:
  root: ./local-bucket      # parent dir for buckets when provider=fs

# gcs:
#   projectID: my-project
#   credentialsFile: ""     # blank = Application Default Credentials

# s3:
#   region: us-east-1
#   endpoint: ""            # set for MinIO and other S3-compatible services
#   usePathStyle: false
```

Values are layered in this order (later wins):

1. The yaml file
2. Environment variables: `BUCKETFILL_PROVIDER`, `BUCKETFILL_BUCKET`, `BUCKETFILL_MIGRATION_DIR`, `BUCKETFILL_FS_ROOT`, `BUCKETFILL_GCS_PROJECT_ID`, `BUCKETFILL_GCS_CREDENTIALS_FILE`, `BUCKETFILL_S3_REGION`, `BUCKETFILL_S3_ENDPOINT`, `BUCKETFILL_S3_USE_PATH_STYLE`, `BUCKETFILL_S3_ACCESS_KEY_ID`, `BUCKETFILL_S3_SECRET_ACCESS_KEY`. Standard AWS env vars (`AWS_REGION`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) are honored as fallbacks.
3. CLI flags: `--provider`, `--bucket`, `--dir`, `--config`

**Secrets do not belong in the yaml file.** Pass S3/GCS credentials via env vars or, for cloud-hosted runs, via your platform's instance/workload identity (ADC for GCS, IAM role for S3).

## Providers

### `fs` — local filesystem

The default. Files land at `<fs.root>/<bucket>/<key>`. Useful for local development, tests, and CI without needing real cloud credentials.

### `gcs` — Google Cloud Storage

Uses [`cloud.google.com/go/storage`](https://pkg.go.dev/cloud.google.com/go/storage). Credentials come from Application Default Credentials by default (env var `GOOGLE_APPLICATION_CREDENTIALS`, `gcloud auth application-default login`, or the host's metadata service). Override with `gcs.credentialsFile` in yaml or `BUCKETFILL_GCS_CREDENTIALS_FILE`.

### `s3` — AWS S3 / S3-compatible

Uses [`aws-sdk-go-v2`](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2). Credentials come from the standard AWS chain (env, shared config, IAM role). Set `s3.endpoint` for MinIO etc. and `s3.usePathStyle: true` if your endpoint requires it.

## Library use

The default flow is to let `bucketfill new` regenerate `cmd/migrate/main.go` for you. If you'd rather drive things by hand — e.g., from your own CLI or as a startup hook — register each version explicitly:

```go
package main

import (
    "context"
    "io/fs"
    "log"

    "github.com/n1kola-petrovic/bucketfill"

    // Blank-import each provider you want available:
    _ "github.com/n1kola-petrovic/bucketfill/provider/gcs"
    _ "github.com/n1kola-petrovic/bucketfill/provider/s3"

    "example.com/myapp/migrations"
    v1 "example.com/myapp/migrations/v1"
    v2 "example.com/myapp/migrations/v2"
)

func main() {
    ctx := context.Background()

    sub := func(p string) fs.FS { s, _ := fs.Sub(migrations.FS, p); return s }

    bucketfill.Register(&bucketfill.Migration{Version: 1, Up: v1.Up, Down: v1.Down, Data: sub("v1/data")})
    bucketfill.Register(&bucketfill.Migration{Version: 2, Up: v2.Up, Down: v2.Down, Data: sub("v2/data")})

    cfg, err := bucketfill.LoadConfig(bucketfill.Overrides{})
    if err != nil {
        log.Fatal(err)
    }
    if err := bucketfill.Up(ctx, cfg); err != nil {
        log.Fatal(err)
    }
}
```

There's also a `bucketfill.RegisterFS(fsys fs.FS)` helper that scans an embed.FS and auto-registers every `v<N>/data` it finds with the default `PutAll`/`DeleteAll` — useful when you don't need per-version Go code at all and want the absolute minimum boilerplate. It skips any version already added via `Register`, so the two patterns mix freely.

Top-level functions: `bucketfill.Up(ctx, cfg)`, `bucketfill.Down(ctx, cfg, DownOpts{To: 0})`, `bucketfill.Status(ctx, cfg)`. For finer control — including plugging in your own structured logger — build the pieces yourself:

```go
storage, _ := bucketfill.OpenProvider(cfg)
client  := bucketfill.NewClient(storage, cfg.EffectiveBucket())
m := bucketfill.NewMigrator(client).WithLogger(myAdapter)
m.Up(ctx)         // or m.DownTo(ctx, 3), or m.Status(ctx)
```

`WithLogger` accepts any `bucketfill.Logger` (a one-method interface: `Logf(format string, args ...any)`). The default writes to `os.Stdout`. A typical adapter for an existing structured logger:

```go
type mylog struct{ l *mypkg.Logger }
func (a mylog) Logf(format string, args ...any) {
    a.l.Info(fmt.Sprintf(format, args...))
}
```

## How it works

State (`{version, appliedAt}`) is stored in the bucket itself as `_bucketfill_state.json`. Up/Down read it, run any pending migration, **persist the new state after each successful step**, and continue. A failure mid-run leaves you with a consistent partial state — the next `bucketfill up` resumes from there.

Versions must be contiguous: registering v1 and v3 with no v2 errors out before any migration runs. This catches mistakes like accidentally deleted folders.

Each version's `data/` subtree is exposed as an `fs.FS` rooted at `migrations/v<N>/data` via `fs.Sub` on the top-level embed. The default `Up`/`Down` walk that subtree to upload/delete; `RegisterFS` is what wires it up at runtime.

The CLI doesn't actually run migrations itself. On the first `bucketfill new` it scaffolds `migrations/embed.go` (the single `//go:embed all:v*`). On every `bucketfill new` / `up` / `down` / `status` it regenerates `cmd/migrate/main.go` to import each `v<N>` package and call `bucketfill.Register` with that package's `Up`/`Down` plus a `fs.Sub(migrations.FS, "v<N>/data")` for the data subtree. Then it runs `go mod tidy` and shells out to `go run ./cmd/migrate <subcommand>`.

That's why adding a new version means just `bucketfill new` + drop files into `data/`: the per-version `up.go`/`down.go` come pre-filled with sensible defaults, the embed glob picks up the new folder at compile time, and the regenerated main.go wires everything together.

## Status

Early. The API may shift before `v1.0`. Issues and feedback welcome.

## License

MIT — see `LICENSE`.
