package contenttype

import (
	"context"
	"testing"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/providers/memory"
)

func TestDetectRecognizesCommonSignatures(t *testing.T) {
	if got := Detect(pngBytes()); got != "image/png" {
		t.Fatalf("png detect = %q", got)
	}
	if got := Detect([]byte("%PDF-1.7")); got != "application/pdf" {
		t.Fatalf("pdf detect = %q", got)
	}
	if got := Detect([]byte("<svg viewBox=\"0 0 1 1\"></svg>")); got != "image/svg+xml" {
		t.Fatalf("svg detect = %q", got)
	}
}

func TestMiddlewareCorrectsDeclaredContentType(t *testing.T) {
	ctx := context.Background()
	client := files.MustNew(files.Options{
		Adapter:    memory.New(memory.Options{}),
		Middleware: []files.Middleware{New(Options{})},
	})
	if _, err := client.Upload(ctx, "photo.bin", files.BytesBody(pngBytes()), files.UploadOptions{ContentType: "text/plain"}); err != nil {
		t.Fatal(err)
	}
	head, err := client.Head(ctx, "photo.bin", files.OperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if head.ContentType != "image/png" {
		t.Fatalf("content type = %q", head.ContentType)
	}
}

func TestMiddlewareRejectsMismatch(t *testing.T) {
	ctx := context.Background()
	client := files.MustNew(files.Options{
		Adapter:    memory.New(memory.Options{}),
		Middleware: []files.Middleware{New(Options{OnMismatch: OnMismatchReject})},
	})
	_, err := client.Upload(ctx, "photo.bin", files.BytesBody(pngBytes()), files.UploadOptions{ContentType: "text/plain"})
	if !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("error = %#v", err)
	}
}

func pngBytes() []byte {
	return []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00}
}
