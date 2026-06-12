package s3compatible

import (
	"context"
	"testing"

	files "github.com/cersho/gofiles-sdk"
)

func TestNewRequiresEndpoint(t *testing.T) {
	_, err := New(context.Background(), Options{
		Bucket:          "bucket",
		Region:          "us-east-1",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
	})
	if !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("error = %#v", err)
	}
}

func TestNewBuildsGenericAdapter(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	adapter, err := New(context.Background(), Options{
		Bucket:          "bucket",
		Region:          "us-east-1",
		Endpoint:        "https://objects.example.test",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
		ForcePathStyle:  true,
		PublicBaseURL:   "https://cdn.example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Name() != "s3-compatible" {
		t.Fatalf("name = %q", adapter.Name())
	}
	if adapter.Bucket() != "bucket" {
		t.Fatalf("bucket = %q", adapter.Bucket())
	}
}
