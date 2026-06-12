package validation

import (
	"context"
	"errors"
	"regexp"
	"testing"

	files "github.com/cersho/gofiles-sdk"
	"github.com/cersho/gofiles-sdk/providers/memory"
)

func TestValidationRejectsKeySizeAndType(t *testing.T) {
	ctx := context.Background()
	maxSize := int64(4)
	client := files.MustNew(files.Options{
		Adapter: memory.New(memory.Options{}),
		Middleware: []files.Middleware{New(Options{
			MaxSize:      &maxSize,
			AllowedTypes: []string{"image/*"},
			KeyPattern:   regexp.MustCompile(`^uploads/`),
		})},
	})

	_, err := client.Upload(ctx, "bad.txt", files.StringBody("png"), files.UploadOptions{ContentType: "image/png"})
	assertReason(t, err, ReasonKey)

	_, err = client.Upload(ctx, "uploads/big.png", files.StringBody("12345"), files.UploadOptions{ContentType: "image/png"})
	assertReason(t, err, ReasonSize)

	_, err = client.Upload(ctx, "uploads/doc.txt", files.StringBody("abc"), files.UploadOptions{ContentType: "text/plain"})
	assertReason(t, err, ReasonType)

	if _, err := client.Upload(ctx, "uploads/icon.png", files.BytesBody([]byte("abc")), files.UploadOptions{ContentType: "image/png"}); err != nil {
		t.Fatal(err)
	}
}

func assertReason(t *testing.T, err error, want Reason) {
	t.Helper()
	if !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("error = %#v", err)
	}
	var validationErr *Error
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected validation error in chain: %#v", err)
	}
	if validationErr.Reason != want {
		t.Fatalf("reason = %q, want %q", validationErr.Reason, want)
	}
}
