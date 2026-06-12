package s3

import (
	"context"
	"testing"
	"time"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
)

func TestNewRequiresBucket(t *testing.T) {
	_, err := New(context.Background(), Options{Region: "us-east-1"})
	if !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("error = %#v", err)
	}
}

func TestPublicURLUsesConfiguredBase(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	adapter, err := New(context.Background(), Options{
		Bucket:              "bucket",
		Region:              "us-east-1",
		PublicBaseURL:       "https://cdn.example.test/assets/",
		DefaultURLExpiresIn: time.Minute,
		Credentials: &Credentials{
			AccessKeyID:     "access",
			SecretAccessKey: "secret",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Name() != "s3" {
		t.Fatalf("name = %q", adapter.Name())
	}
	got, err := adapter.URL(context.Background(), "a folder/file.txt", files.URLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://cdn.example.test/assets/a%20folder/file.txt"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}
