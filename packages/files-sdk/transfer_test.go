package files_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/providers/memory"
)

func TestTransferCopiesBetweenClientsAndSkipsExisting(t *testing.T) {
	ctx := context.Background()
	source := files.MustNew(files.Options{Adapter: memory.New(memory.Options{})})
	dest := files.MustNew(files.Options{Adapter: memory.New(memory.Options{})})

	if _, err := source.Upload(ctx, "docs/a.txt", files.StringBody("alpha"), files.UploadOptions{ContentType: "text/plain"}); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Upload(ctx, "docs/b.txt", files.StringBody("bravo"), files.UploadOptions{ContentType: "text/plain"}); err != nil {
		t.Fatal(err)
	}
	if _, err := dest.Upload(ctx, "backup/b.txt", files.StringBody("existing"), files.UploadOptions{ContentType: "text/plain"}); err != nil {
		t.Fatal(err)
	}

	overwrite := false
	var progress []files.TransferProgress
	result, err := files.Transfer(ctx, source, dest, files.TransferOptions{
		Prefix:    "docs/",
		Overwrite: &overwrite,
		TransformKey: func(key string) string {
			return "backup/" + strings.TrimPrefix(key, "docs/")
		},
		OnProgress: func(p files.TransferProgress) {
			progress = append(progress, p)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Transferred, []string{"docs/a.txt"}) {
		t.Fatalf("transferred = %#v", result.Transferred)
	}
	if !reflect.DeepEqual(result.Skipped, []string{"docs/b.txt"}) {
		t.Fatalf("skipped = %#v", result.Skipped)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("errors = %#v", result.Errors)
	}
	if len(progress) != 2 || progress[len(progress)-1].Done != 2 {
		t.Fatalf("progress = %#v", progress)
	}

	file, err := dest.Download(ctx, "backup/a.txt", files.DownloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	text, err := file.Text(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if text != "alpha" {
		t.Fatalf("transferred text = %q", text)
	}
}
