package r2

import (
	"context"
	"testing"

	files "github.com/cersho/gofiles-sdk"
)

func TestNewRequiresAccountID(t *testing.T) {
	_, err := New(context.Background(), Options{
		Bucket:          "bucket",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
	})
	if !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("error = %#v", err)
	}
}

func TestR2HTTPAdapterRejectsPostPolicyUpload(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	adapter, err := New(context.Background(), Options{
		Bucket:          "bucket",
		AccountID:       "account",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Name() != "r2-http" {
		t.Fatalf("name = %q", adapter.Name())
	}
	maxSize := int64(1024)
	_, err = adapter.SignedUploadURL(context.Background(), "a.txt", files.SignedUploadOptions{MaxSize: &maxSize})
	if !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("error = %#v", err)
	}
}
