# Go Files SDK Monorepo

This repository contains the Go Files SDK package and its documentation site.

## Layout

- `packages/files-sdk` - Go module for the SDK.
- `apps/web` - documentation site.

## Install

```bash
go get github.com/cersho/gofiles-sdk/packages/files-sdk
```

Provider packages are imported from the same module path:

```go
import (
	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/providers/s3"
)
```

## Development

Run Go commands through the root scripts:

```bash
bun run go:download
bun run go:fmt:check
bun run go:build
bun run go:vet
bun run go:test
```

Docs commands also run from the repository root:

```bash
bun install
bun run docs:dev
bun run docs:types
bun run docs:build
```

## Publishing

Go releases use submodule-style tags so the Go proxy resolves the module below
`packages/files-sdk`:

```bash
git tag packages/files-sdk/v0.1.0
git push origin packages/files-sdk/v0.1.0
```

The publish workflow validates the Go module, builds the docs, creates a GitHub
Release, and deploys the docs site.

## License

[MIT](packages/files-sdk/LICENSE)
