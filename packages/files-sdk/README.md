# Go Files SDK

A unified Go API for object storage backends such as S3, Cloudflare R2, local files, memory storage, and UploadThing.

## Highlights

- **One client shape:** use the same methods for upload, download, list, copy, move, signed URLs, and deletes.
- **Go-native I/O:** pass `io.Reader`, bytes, strings, or files through the `files.Body` helpers.
- **Provider packages:** import only the backend you use, such as `providers/s3`, `providers/r2`, or `providers/fs`.
- **Normalized errors:** handle `*files.Error` codes instead of provider-specific error envelopes.
- **Operational hooks:** observe actions, errors, retries, upload progress, and transfers without wrapping every call.

## Install

```bash
go get github.com/cersho/gofiles-sdk/packages/files-sdk
```

Requires Go 1.26 or newer.

## Quick Start

```go
package main

import (
	"context"
	"fmt"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/providers/memory"
)

func main() {
	ctx := context.Background()
	client := files.MustNew(files.Options{
		Adapter: memory.New(memory.Options{}),
	})

	_, err := client.Upload(ctx, "reports/q1.txt", files.StringBody("revenue: 42"), files.UploadOptions{
		ContentType: "text/plain",
	})
	if err != nil {
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

## Usage

Create a client with a provider adapter:

```go
s3Adapter, err := s3.New(ctx, s3.Options{
	Bucket: "uploads",
	Region: "us-east-1",
})
if err != nil {
	return err
}

client, err := files.New(files.Options{Adapter: s3Adapter})
if err != nil {
	return err
}
```

Use the same client methods with every provider:

```go
_, err = client.Upload(ctx, "avatars/ada.png", files.FileBody("ada.png"), files.UploadOptions{
	ContentType: "image/png",
	Metadata: map[string]string{
		"user-id": "user_123",
	},
})
if err != nil {
	return err
}

exists, err := client.Exists(ctx, "avatars/ada.png", files.OperationOptions{})
if err != nil {
	return err
}
fmt.Println(exists)
```

## API

The root package exposes `Client`, `Options`, body helpers, normalized errors, hooks, middleware, bulk result types, and transfer helpers.

Provider packages live under `github.com/cersho/gofiles-sdk/packages/files-sdk/providers/...`. Plugin packages live under `github.com/cersho/gofiles-sdk/packages/files-sdk/plugins/...`.

For full usage guides, run the docs site:

```bash
bun install
bun run docs:dev
```

## Development

```bash
go test ./...
go vet ./...
gofmt -w .
```

Docs commands run from the repository root:

```bash
bun run docs:types
bun run docs:build
```

## License

[MIT](LICENSE)
