package s3compatible

import (
	"context"
	"os"
	"time"

	files "github.com/cersho/gofiles-sdk"
	"github.com/cersho/gofiles-sdk/internal/envutil"
	s3provider "github.com/cersho/gofiles-sdk/providers/s3"
)

type Options struct {
	Name                 string
	Bucket               string
	Region               string
	Endpoint             string
	AccessKeyID          string
	SecretAccessKey      string
	SessionToken         string
	ForcePathStyle       bool
	PublicBaseURL        string
	DefaultURLExpiresIn  time.Duration
	DefaultProviderLabel string
}

func New(ctx context.Context, opts Options) (*s3provider.Adapter, error) {
	name := opts.Name
	if name == "" {
		name = "s3-compatible"
	}
	if opts.Endpoint == "" {
		return nil, files.NewError(files.ErrProvider, name+" adapter: missing endpoint", nil)
	}
	region := opts.Region
	if region == "" {
		region = "us-east-1"
	}
	accessKeyID := envutil.First(opts.AccessKeyID, os.Getenv("S3_COMPATIBLE_ACCESS_KEY_ID"))
	secretAccessKey := envutil.First(opts.SecretAccessKey, os.Getenv("S3_COMPATIBLE_SECRET_ACCESS_KEY"))
	if accessKeyID == "" || secretAccessKey == "" {
		return nil, files.NewError(files.ErrProvider, name+" adapter: missing credentials. Pass AccessKeyID + SecretAccessKey or set S3_COMPATIBLE_ACCESS_KEY_ID + S3_COMPATIBLE_SECRET_ACCESS_KEY.", nil)
	}
	label := opts.DefaultProviderLabel
	if label == "" {
		label = name + " error"
	}
	return s3provider.NewNamed(ctx, name, s3provider.Options{
		Bucket:               opts.Bucket,
		Region:               region,
		Endpoint:             opts.Endpoint,
		ForcePathStyle:       opts.ForcePathStyle,
		PublicBaseURL:        opts.PublicBaseURL,
		DefaultURLExpiresIn:  opts.DefaultURLExpiresIn,
		DefaultProviderLabel: label,
		Credentials: &s3provider.Credentials{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
			SessionToken:    opts.SessionToken,
		},
	})
}
