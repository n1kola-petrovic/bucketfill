# bucketfill

Versioned seeds and migrations for object storage — like `golang-migrate` for SQL, but for buckets. Works with the local filesystem, Google Cloud Storage, and AWS S3 / S3-compatible services.

Each migration is just a folder. You drop files into `migrations/v<N>/data/` and the nesting under `data/` mirrors the bucket layout. Apply them with `bucketfill up`, roll back with `bucketfill down`. State is tracked inside the bucket itself, so a fresh checkout of your repo plus access to the bucket is enough to know which migrations have run.

There's no per-version Go code to write and no main.go to edit when adding a new version — the runtime auto-discovers every `v<N>/` from a single embed.FS. Custom Up/Down logic remains opt-in for the rare cases that need it.

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

bucketfill new              # creates migrations/v1/data/ (and the one-time
                            # migrations/embed.go + cmd/migrate/main.go)
# drop your assets into migrations/v1/data/<wherever you want them in the bucket>
bucketfill up               # apply all pending migrations

bucketfill new              # next time you need a v2 — just creates v2/data/
# drop files into migrations/v2/data/
bucketfill up               # picks up v2 automatically; no code edits needed

bucketfill status           # see what's applied vs pending
bucketfill down             # roll back the most recent migration
bucketfill down --to 0      # roll back everything
```

The first `bucketfill new` also writes two static files you'll commit:
`migrations/embed.go` (one `//go:embed all:v*` glob) and `cmd/migrate/main.go`
(calls `bucketfill.RegisterFS`). Neither is regenerated afterward — adding new
versions just means adding folders.

`bucketfill init` only writes the yaml. The migrations directory is created the first time you run `bucketfill new`, using whatever `migrationDir` says in your config — so feel free to rename it from `migrations` to `db/seeds`, `bucket-migrations`, or whatever fits your project.

## Project layout

After `bucketfill new`, a project looks like:

```
my-project/
├── bucketfill.yaml            # provider, bucket, migrationDir
├── go.mod
├── migrations/
│   ├── embed.go               # one file, scaffolded once; //go:embed all:v*
│   ├── v1/
│   │   └── data/              # nesting mirrors the bucket layout
│   │       └── seeds/
│   │           └── welcome.txt
│   └── v2/
│       └── data/
│           └── ...
└── cmd/migrate/
    └── main.go                # one file, scaffolded once; calls RegisterFS
```

**Adding a new migration version is just:**

```bash
bucketfill new          # creates migrations/v<N+1>/data/.keep
# drop your files into migrations/v<N+1>/data/
bucketfill up
```

No Go files to write, no main.go to edit, no per-version registration. Each `v<N>/` is just a folder of files. `migrations/embed.go` glob-matches every `v*` directory, and `cmd/migrate/main.go` walks the embed.FS at runtime via `bucketfill.RegisterFS`, auto-registering each version with `c.PutAll(ctx)` for Up and `c.DeleteAll(ctx)` for Down.

`migrations/embed.go` and `cmd/migrate/main.go` are static — bucketfill creates them once and never touches them again. Commit them like normal source files.

## Writing a custom migration

The default behavior — "mirror data/ into the bucket on Up, mirror it back out on Down" — covers the typical case. If you need custom logic for a specific version (a rename, a conditional, a one-off transform), opt in:

1. Add a Go file to that version's folder, e.g. `migrations/v3/migration.go`:
   ```go
   package v3

   import (
       "context"

       "github.com/n1kola-petrovic/bucketfill"
   )

   func Up(ctx context.Context, c *bucketfill.Client) error {
       if err := c.PutAll(ctx); err != nil {
           return err
       }
       return c.Rename(ctx, "old/key.txt", "new/key.txt")
   }

   func Down(ctx context.Context, c *bucketfill.Client) error {
       return c.DeleteAll(ctx)
   }
   ```

2. Register it explicitly in `cmd/migrate/main.go` *before* `RegisterFS`:
   ```go
   import (
       // ... existing imports ...
       "io/fs"

       "example.com/my-project/migrations"
       v3 "example.com/my-project/migrations/v3"
   )

   func main() {
       sub, _ := fs.Sub(migrations.FS, "v3/data")
       bucketfill.Register(&bucketfill.Migration{
           Version: 3,
           Up:      v3.Up,
           Down:    v3.Down,
           Data:    sub,
       })
       if err := bucketfill.RegisterFS(migrations.FS); err != nil { /* ... */ }
       // ... RunCLI ...
   }
   ```

`RegisterFS` skips versions already in the registry, so `v3` keeps your custom logic while `v1`, `v2`, `v4`, ... continue to use the defaults.

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

If you'd rather drive bucketfill programmatically — e.g., run migrations from your own CLI or as a startup hook — point `RegisterFS` at your migrations embed.FS and call `bucketfill.Up`:

```go
package main

import (
    "context"
    "log"

    "github.com/n1kola-petrovic/bucketfill"

    // Blank-import each provider you want available:
    _ "github.com/n1kola-petrovic/bucketfill/provider/gcs"
    _ "github.com/n1kola-petrovic/bucketfill/provider/s3"

    "example.com/myapp/migrations"
)

func main() {
    ctx := context.Background()

    if err := bucketfill.RegisterFS(migrations.FS); err != nil {
        log.Fatal(err)
    }

    cfg, err := bucketfill.LoadConfig(bucketfill.Overrides{})
    if err != nil {
        log.Fatal(err)
    }

    if err := bucketfill.Up(ctx, cfg); err != nil {
        log.Fatal(err)
    }
}
```

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

The CLI doesn't actually run migrations itself. On the first `bucketfill new` it scaffolds two static files in your project — `migrations/embed.go` (one `//go:embed all:v*` glob) and `cmd/migrate/main.go` (a tiny binary that calls `bucketfill.RegisterFS`). On every `up`/`down`/`status` it runs `go mod tidy` and shells out to `go run ./cmd/migrate <subcommand>`. The compiled binary opens the embed.FS, walks it for `v<N>/data` subtrees, and registers each as a default migration with Up=`PutAll` and Down=`DeleteAll`.

That's why adding a new version requires zero code edits: the embed glob picks up the new directory at compile time, and the runtime registration walks whatever's there.

## Status

Early. The API may shift before `v1.0`. Issues and feedback welcome.

## License

MIT — see `LICENSE`.
