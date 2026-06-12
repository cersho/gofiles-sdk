package r2

import (
	"context"
	"os"
	"time"

	files "github.com/cersho/gofiles-sdk"
	"github.com/cersho/gofiles-sdk/internal/envutil"
	s3provider "github.com/cersho/gofiles-sdk/providers/s3"
)

type Options struct {
	Bucket              string
	AccountID           string
	AccessKeyID         string
	SecretAccessKey     string
	PublicBaseURL       string
	DefaultURLExpiresIn time.Duration
}

type Adapter struct {
	inner *s3provider.Adapter
}

func New(ctx context.Context, opts Options) (*Adapter, error) {
	accountID := envutil.First(opts.AccountID, os.Getenv("R2_ACCOUNT_ID"))
	accessKeyID := envutil.First(opts.AccessKeyID, os.Getenv("R2_ACCESS_KEY_ID"))
	secretAccessKey := envutil.First(opts.SecretAccessKey, os.Getenv("R2_SECRET_ACCESS_KEY"))
	if accountID == "" {
		return nil, files.NewError(files.ErrProvider, "r2 adapter: missing accountId. Pass AccountID or set R2_ACCOUNT_ID.", nil)
	}
	if accessKeyID == "" || secretAccessKey == "" {
		return nil, files.NewError(files.ErrProvider, "r2 adapter: missing credentials. Pass AccessKeyID + SecretAccessKey or set R2_ACCESS_KEY_ID + R2_SECRET_ACCESS_KEY.", nil)
	}
	inner, err := s3provider.NewNamed(ctx, "r2-http", s3provider.Options{
		Bucket:               opts.Bucket,
		Region:               "auto",
		Endpoint:             "https://" + accountID + ".r2.cloudflarestorage.com",
		ForcePathStyle:       true,
		PublicBaseURL:        opts.PublicBaseURL,
		DefaultURLExpiresIn:  opts.DefaultURLExpiresIn,
		DefaultProviderLabel: "R2 error",
		Credentials: &s3provider.Credentials{
			AccessKeyID:     accessKeyID,
			SecretAccessKey: secretAccessKey,
		},
	})
	if err != nil {
		return nil, err
	}
	return &Adapter{inner: inner}, nil
}

func (a *Adapter) Name() string { return a.inner.Name() }

func (a *Adapter) Raw() any { return a.inner.Raw() }

func (a *Adapter) Capabilities() files.AdapterCapabilities { return a.inner.Capabilities() }

func (a *Adapter) Upload(ctx context.Context, key string, body files.Body, opts files.UploadOptions) (files.UploadResult, error) {
	return a.inner.Upload(ctx, key, body, opts)
}

func (a *Adapter) Download(ctx context.Context, key string, opts files.DownloadOptions) (files.StoredFile, error) {
	return a.inner.Download(ctx, key, opts)
}

func (a *Adapter) Head(ctx context.Context, key string, opts files.OperationOptions) (files.StoredFile, error) {
	return a.inner.Head(ctx, key, opts)
}

func (a *Adapter) Exists(ctx context.Context, key string, opts files.OperationOptions) (bool, error) {
	return a.inner.Exists(ctx, key, opts)
}

func (a *Adapter) Delete(ctx context.Context, key string, opts files.OperationOptions) error {
	return a.inner.Delete(ctx, key, opts)
}

func (a *Adapter) DeleteMany(ctx context.Context, keys []string, opts files.DeleteManyOptions) (files.DeleteManyResult, error) {
	return a.inner.DeleteMany(ctx, keys, opts)
}

func (a *Adapter) Copy(ctx context.Context, from string, to string, opts files.OperationOptions) error {
	return a.inner.Copy(ctx, from, to, opts)
}

func (a *Adapter) List(ctx context.Context, opts files.ListOptions) (files.ListResult, error) {
	return a.inner.List(ctx, opts)
}

func (a *Adapter) URL(ctx context.Context, key string, opts files.URLOptions) (string, error) {
	return a.inner.URL(ctx, key, opts)
}

func (a *Adapter) SignedUploadURL(ctx context.Context, key string, opts files.SignedUploadOptions) (files.SignedUpload, error) {
	if opts.MaxSize != nil {
		return files.SignedUpload{}, files.NewError(files.ErrProvider, "r2: MaxSize is not supported because Cloudflare R2 does not implement the S3 POST Object API.", nil)
	}
	return a.inner.SignedUploadURL(ctx, key, opts)
}
