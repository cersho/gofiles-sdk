package digitaloceanspaces

import (
	"context"
	"testing"

	files "github.com/cersho/gofiles-sdk"
)

func TestNewRequiresRegion(t *testing.T) {
	_, err := New(context.Background(), Options{
		Bucket:          "bucket",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
	})
	if !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("error = %#v", err)
	}
}

func TestNewConfiguresSpacesAdapter(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	adapter, err := New(context.Background(), Options{
		Bucket:          "bucket",
		Region:          "nyc3",
		AccessKeyID:     "access",
		SecretAccessKey: "secret",
		PublicBaseURL:   "https://assets.example.test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.Name() != "digitalocean-spaces" {
		t.Fatalf("name = %q", adapter.Name())
	}
	got, err := adapter.URL(context.Background(), "img/logo.png", files.URLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://assets.example.test/img/logo.png"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
}
