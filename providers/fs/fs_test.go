package fs

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	files "github.com/cersho/gofiles-sdk"
)

func TestAdapterPersistsFilesMetadataRangeAndURL(t *testing.T) {
	ctx := context.Background()
	adapter, err := New(Options{Root: t.TempDir(), URLBaseURL: "https://static.example.test/files"})
	if err != nil {
		t.Fatal(err)
	}
	client := files.MustNew(files.Options{Adapter: adapter})

	out, err := client.Upload(ctx, "dir/a.txt", files.StringBody("abcdef"), files.UploadOptions{
		ContentType: "text/plain",
		Metadata:    map[string]string{"owner": "tests"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Size != 6 || out.ContentType != "text/plain" {
		t.Fatalf("upload result = %#v", out)
	}
	if _, err := os.Stat(filepath.Join(adapter.Root(), "dir", "a.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(adapter.Root(), "dir", "a.txt"+sidecarSuffix)); err != nil {
		t.Fatal(err)
	}

	end := int64(3)
	file, err := client.Download(ctx, "dir/a.txt", files.DownloadOptions{Range: &files.ByteRange{Start: 1, End: &end}})
	if err != nil {
		t.Fatal(err)
	}
	text, err := file.Text(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if text != "bcd" {
		t.Fatalf("range text = %q", text)
	}

	head, err := client.Head(ctx, "dir/a.txt", files.OperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if head.Metadata["owner"] != "tests" {
		t.Fatalf("metadata = %#v", head.Metadata)
	}

	gotURL, err := client.URL(ctx, "dir/a.txt", files.URLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if gotURL != "https://static.example.test/files/dir/a.txt" {
		t.Fatalf("url = %q", gotURL)
	}
}

func TestAdapterRejectsEscapingAndReservedKeys(t *testing.T) {
	ctx := context.Background()
	adapter, err := New(Options{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Upload(ctx, "../escape.txt", files.StringBody("x"), files.UploadOptions{}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("escape upload error = %#v", err)
	}
	if _, err := adapter.SignedUploadURL(ctx, "../escape.txt", files.SignedUploadOptions{}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("escape signed upload error = %#v", err)
	}
	if _, err := adapter.Upload(ctx, "x.meta.json", files.StringBody("x"), files.UploadOptions{}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("reserved key error = %#v", err)
	}
}

func TestResumableUploadThroughClient(t *testing.T) {
	ctx := context.Background()
	adapter, err := New(Options{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	client := files.MustNew(files.Options{Adapter: adapter})
	control := files.NewUploadControl()
	data := []byte("filesystem resumable upload")

	out, err := client.Upload(ctx, "large.bin", files.BytesBody(data), files.UploadOptions{
		ContentType: "application/octet-stream",
		Control:     control,
		Multipart:   &files.MultipartOptions{PartSize: 7},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Size != int64(len(data)) || control.Status() != files.UploadControlCompleted {
		t.Fatalf("out/status = %#v/%s", out, control.Status())
	}

	file, err := client.Download(ctx, "large.bin", files.DownloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	body, err := file.Bytes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(data) {
		t.Fatalf("downloaded body = %q", string(body))
	}
}
