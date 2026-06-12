package files

import "context"

type OperationKind string

const (
	OperationUpload          OperationKind = "upload"
	OperationDownload        OperationKind = "download"
	OperationHead            OperationKind = "head"
	OperationExists          OperationKind = "exists"
	OperationDelete          OperationKind = "delete"
	OperationCopy            OperationKind = "copy"
	OperationMove            OperationKind = "move"
	OperationList            OperationKind = "list"
	OperationURL             OperationKind = "url"
	OperationSignedUploadURL OperationKind = "signedUploadUrl"
)

type Operation struct {
	Kind                OperationKind
	Key                 string
	Keys                []string
	From                string
	To                  string
	Body                Body
	Bulk                bool
	UploadOptions       UploadOptions
	DownloadOptions     DownloadOptions
	OperationOptions    OperationOptions
	ListOptions         ListOptions
	URLOptions          URLOptions
	SignedUploadOptions SignedUploadOptions
}

type Handler func(context.Context, Operation) (any, error)
type Middleware func(context.Context, Operation, Handler) (any, error)

func Handlers(handlers map[OperationKind]Middleware) Middleware {
	return func(ctx context.Context, op Operation, next Handler) (any, error) {
		if h := handlers[op.Kind]; h != nil {
			return h(ctx, op, next)
		}
		return next(ctx, op)
	}
}
