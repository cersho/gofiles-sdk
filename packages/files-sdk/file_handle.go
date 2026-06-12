package files

import "context"

type FileHandle struct {
	client *Client
	key    string
}

func (f FileHandle) Key() string {
	return f.key
}

func (f FileHandle) Upload(ctx context.Context, body Body, opts UploadOptions) (UploadResult, error) {
	return f.client.Upload(ctx, f.key, body, opts)
}

func (f FileHandle) Download(ctx context.Context, opts DownloadOptions) (StoredFile, error) {
	return f.client.Download(ctx, f.key, opts)
}

func (f FileHandle) Head(ctx context.Context, opts OperationOptions) (StoredFile, error) {
	return f.client.Head(ctx, f.key, opts)
}

func (f FileHandle) Exists(ctx context.Context, opts OperationOptions) (bool, error) {
	return f.client.Exists(ctx, f.key, opts)
}

func (f FileHandle) Delete(ctx context.Context, opts OperationOptions) error {
	return f.client.Delete(ctx, f.key, opts)
}

func (f FileHandle) URL(ctx context.Context, opts URLOptions) (string, error) {
	return f.client.URL(ctx, f.key, opts)
}

func (f FileHandle) SignedUploadURL(ctx context.Context, opts SignedUploadOptions) (SignedUpload, error) {
	return f.client.SignedUploadURL(ctx, f.key, opts)
}

func (f FileHandle) CopyTo(ctx context.Context, destination string, opts OperationOptions) error {
	return f.client.Copy(ctx, f.key, destination, opts)
}

func (f FileHandle) CopyFrom(ctx context.Context, source string, opts OperationOptions) error {
	return f.client.Copy(ctx, source, f.key, opts)
}

func (f FileHandle) MoveTo(ctx context.Context, destination string, opts OperationOptions) error {
	return f.client.Move(ctx, f.key, destination, opts)
}

func (f FileHandle) MoveFrom(ctx context.Context, source string, opts OperationOptions) error {
	return f.client.Move(ctx, source, f.key, opts)
}
