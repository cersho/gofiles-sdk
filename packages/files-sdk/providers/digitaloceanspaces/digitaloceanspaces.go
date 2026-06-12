package digitaloceanspaces

import (
	"context"
	"os"
	"time"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/envutil"
	s3provider "github.com/cersho/gofiles-sdk/packages/files-sdk/providers/s3"
)

type Options struct {
	Bucket              string
	Region              string
	Endpoint            string
	AccessKeyID         string
	SecretAccessKey     string
	ForcePathStyle      bool
	PublicBaseURL       string
	DefaultURLExpiresIn time.Duration
}

func New(ctx context.Context, opts Options) (*s3provider.Adapter, error) {
	if opts.Region == "" {
		return nil, files.NewError(files.ErrProvider, "digitalocean-spaces adapter: missing region. Pass Region, for example nyc3.", nil)
	}
	accessKeyID := envutil.First(opts.AccessKeyID, os.Getenv("DO_SPACES_KEY"))
	secretAccessKey := envutil.First(opts.SecretAccessKey, os.Getenv("DO_SPACES_SECRET"))
	if accessKeyID == "" || secretAccessKey == "" {
		return nil, files.NewError(files.ErrProvider, "digitalocean-spaces adapter: missing credentials. Pass AccessKeyID + SecretAccessKey or set DO_SPACES_KEY + DO_SPACES_SECRET.", nil)
	}
	endpoint := opts.Endpoint
	if endpoint == "" {
		endpoint = "https://" + opts.Region + ".digitaloceanspaces.com"
	}
	return s3provider.NewNamed(ctx, "digitalocean-spaces", s3provider.Options{
		Bucket:               opts.Bucket,
		Region:               opts.Region,
		Endpoint:             endpoint,
		ForcePathStyle:       opts.ForcePathStyle,
		PublicBaseURL:        opts.PublicBaseURL,
		DefaultURLExpiresIn:  opts.DefaultURLExpiresIn,
		DefaultProviderLabel: "Spaces error",
		Credentials: &s3provider.Credentials{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
		},
	})
}
