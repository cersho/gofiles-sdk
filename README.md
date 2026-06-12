# Go Files SDK

A unified storage SDK for Go services that need one client shape across object and blob storage providers.

## Highlights

- **One API across providers:** call `Upload`, `Download`, `Head`, `Exists`, `Delete`, `Copy`, `Move`, `List`, `ListAll`, `Search`, `URL`, and `SignedUploadURL` against any adapter.
- **Go-native bodies:** upload strings, byte slices, readers, files, or custom read closers with the `files.Body` helpers.
- **File handles:** bind repeated operations to one key with `client.File("backups/backup.sql")`.
- **Operational controls:** configure prefixes, read-only clients, timeouts, retries, hooks, middleware, progress, bulk work, and transfers.
- **Provider escape hatch:** use `client.Raw()` when provider-specific behavior belongs in the native client.

## Install

```bash
go get github.com/cersho/gofiles-sdk
```

Requires Go 1.26 or newer.

## Quick Start

```go
package main

import (
	"context"
	"fmt"

	files "github.com/cersho/gofiles-sdk"
	"github.com/cersho/gofiles-sdk/providers/memory"
)

func main() {
	ctx := context.Background()
	client := files.MustNew(files.Options{Adapter: memory.New(memory.Options{})})

	if _, err := client.Upload(ctx, "reports/q1.txt", files.StringBody("revenue: 42"), files.UploadOptions{ContentType: "text/plain"}); err != nil {
		panic(err)
	}

	stored, err := client.Download(ctx, "reports/q1.txt", files.DownloadOptions{})
	if err != nil {
		panic(err)
	}

	text, err := stored.Text(ctx)
	if err != nil {
		panic(err)
	}

	fmt.Println(text)
	// Output: revenue: 42
}
```

Save it as `main.go`, run `go run .`, then swap the memory adapter for a cloud provider when you are ready.

## Provider Packages

Import only the adapter you use:

| Backend | Import path |
| --- | --- |
| Memory | `github.com/cersho/gofiles-sdk/providers/memory` |
| Filesystem | `github.com/cersho/gofiles-sdk/providers/fs` |
| S3 | `github.com/cersho/gofiles-sdk/providers/s3` |
| Cloudflare R2 | `github.com/cersho/gofiles-sdk/providers/r2` |
| S3-compatible storage | `github.com/cersho/gofiles-sdk/providers/s3compatible` |
| DigitalOcean Spaces | `github.com/cersho/gofiles-sdk/providers/digitaloceanspaces` |
| UploadThing | `github.com/cersho/gofiles-sdk/providers/uploadthing` |
| Vercel Blob | `github.com/cersho/gofiles-sdk/providers/vercelblob` |

S3-backed adapters use AWS SDK for Go v2, which is already declared by the module.

## What You Get

- Client creation with `files.New` or `files.MustNew`, plus upload inputs from `StringBody`, `BytesBody`, `ReaderBody`, `FileBody`, or `NewBodyFromReadCloser`.
- Key-scoped operations with `client.File(key)`, bounded-concurrency work with the `*Many` methods, and cross-client copies with `files.Transfer`.
- Normalized `*files.Error` values with `NotFound`, `Unauthorized`, `Conflict`, `ReadOnly`, and `Provider` codes.

## Development

Run these commands from the repository root:

```bash
bun install
bun run test
bun run build
bun run docs:dev
```

## License

[MIT](LICENSE)
