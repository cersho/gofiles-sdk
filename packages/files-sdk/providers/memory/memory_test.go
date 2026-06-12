package memory

import (
	"context"
	"testing"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
)

func TestAdapterCRUDListRangeAndURL(t *testing.T) {
	ctx := context.Background()
	client := files.MustNew(files.Options{Adapter: New(Options{
		Initial: map[string]Seed{
			"seed.txt": {Body: []byte("seed"), ContentType: "text/plain"},
		},
	})})

	out, err := client.Upload(ctx, "dir/a.txt", files.StringBody("abcdef"), files.UploadOptions{
		ContentType: "text/plain",
		Metadata:    map[string]string{"owner": "tests"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Key != "dir/a.txt" || out.Size != 6 || out.ContentType != "text/plain" {
		t.Fatalf("upload result = %#v", out)
	}

	if _, err := client.Upload(ctx, "dir/nested/b.txt", files.StringBody("nested"), files.UploadOptions{}); err != nil {
		t.Fatal(err)
	}

	end := int64(2)
	file, err := client.Download(ctx, "dir/a.txt", files.DownloadOptions{Range: &files.ByteRange{Start: 1, End: &end}})
	if err != nil {
		t.Fatal(err)
	}
	text, err := file.Text(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if text != "bc" {
		t.Fatalf("range text = %q", text)
	}

	head, err := client.Head(ctx, "dir/a.txt", files.OperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if head.Metadata["owner"] != "tests" {
		t.Fatalf("metadata = %#v", head.Metadata)
	}

	list, err := client.List(ctx, files.ListOptions{Prefix: "dir/", Delimiter: "/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Key != "dir/a.txt" {
		t.Fatalf("items = %#v", list.Items)
	}
	if len(list.Prefixes) != 1 || list.Prefixes[0] != "dir/nested/" {
		t.Fatalf("prefixes = %#v", list.Prefixes)
	}

	gotURL, err := client.URL(ctx, "dir/a.txt", files.URLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if gotURL != "memory://dir%2Fa.txt" {
		t.Fatalf("url = %q", gotURL)
	}

	if err := client.Copy(ctx, "dir/a.txt", "dir/copy.txt", files.OperationOptions{}); err != nil {
		t.Fatal(err)
	}
	if err := client.Move(ctx, "dir/copy.txt", "dir/moved.txt", files.OperationOptions{}); err != nil {
		t.Fatal(err)
	}
	exists, err := client.Exists(ctx, "dir/moved.txt", files.OperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatal("moved object does not exist")
	}
}

func TestResumableUploadThroughClient(t *testing.T) {
	ctx := context.Background()
	client := files.MustNew(files.Options{Adapter: New(Options{})})
	control := files.NewUploadControl()
	data := []byte("hello resumable upload")
	var progress []files.UploadProgress

	out, err := client.Upload(ctx, "large.bin", files.BytesBody(data), files.UploadOptions{
		ContentType: "application/octet-stream",
		Control:     control,
		Multipart:   &files.MultipartOptions{PartSize: 5},
		OnProgress: func(p files.UploadProgress) {
			progress = append(progress, p)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Size != int64(len(data)) {
		t.Fatalf("size = %d", out.Size)
	}
	if control.Status() != files.UploadControlCompleted {
		t.Fatalf("status = %s", control.Status())
	}
	if control.Loaded() != int64(len(data)) || control.Total() != int64(len(data)) {
		t.Fatalf("loaded/total = %d/%d", control.Loaded(), control.Total())
	}
	if len(progress) == 0 || progress[len(progress)-1].Loaded != int64(len(data)) {
		t.Fatalf("progress = %#v", progress)
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
