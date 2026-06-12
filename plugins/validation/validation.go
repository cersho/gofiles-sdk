package validation

import (
	"context"
	"mime"
	"path/filepath"
	"regexp"
	"strings"

	files "github.com/cersho/gofiles-sdk"
)

type Reason string

const (
	ReasonKey  Reason = "key"
	ReasonSize Reason = "size"
	ReasonType Reason = "type"
)

type Error struct {
	Reason  Reason
	Message string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type Options struct {
	MaxSize      *int64
	MinSize      *int64
	AllowedTypes []string
	KeyPattern   *regexp.Regexp
	KeyFunc      func(string) bool
}

func New(opts Options) files.Middleware {
	return func(ctx context.Context, op files.Operation, next files.Handler) (any, error) {
		switch op.Kind {
		case files.OperationUpload:
			if err := validateKey(opts, op.Key); err != nil {
				return nil, err
			}
			uploadOpts := op.UploadOptions
			body := op.Body
			if len(opts.AllowedTypes) > 0 {
				contentType := resolveType(op.Key, body, uploadOpts.ContentType)
				if !typeAllowed(contentType, opts.AllowedTypes) {
					return nil, newError(ReasonType, `validation: "`+op.Key+`" has type "`+baseType(contentType)+`", which is not allowed`)
				}
			}
			if opts.MaxSize != nil || opts.MinSize != nil {
				size, ok := body.Size()
				if !ok {
					data, err := body.ReadAll(ctx)
					if err != nil {
						return nil, err
					}
					size = int64(len(data))
					body = files.BytesBody(data)
					if uploadOpts.ContentType == "" {
						uploadOpts.ContentType = resolveType(op.Key, op.Body, "")
					}
				}
				if err := validateSize(opts, op.Key, size); err != nil {
					return nil, err
				}
			}
			nextOp := op
			nextOp.Body = body
			nextOp.UploadOptions = uploadOpts
			return next(ctx, nextOp)
		case files.OperationCopy:
			if err := validateKey(opts, op.To); err != nil {
				return nil, err
			}
		case files.OperationMove:
			if err := validateKey(opts, op.To); err != nil {
				return nil, err
			}
		case files.OperationSignedUploadURL:
			if err := validateKey(opts, op.Key); err != nil {
				return nil, err
			}
			if opts.MaxSize != nil || opts.MinSize != nil || len(opts.AllowedTypes) > 0 {
				return nil, files.NewError(files.ErrProvider, "validation: signed upload URLs bypass size and type checks; upload through the Files client to enforce them", nil)
			}
		}
		return next(ctx, op)
	}
}

func validateKey(opts Options, key string) error {
	if opts.KeyPattern != nil && !opts.KeyPattern.MatchString(key) {
		return newError(ReasonKey, `validation: key "`+key+`" is not allowed`)
	}
	if opts.KeyFunc != nil && !opts.KeyFunc(key) {
		return newError(ReasonKey, `validation: key "`+key+`" is not allowed`)
	}
	return nil
}

func validateSize(opts Options, key string, size int64) error {
	if opts.MaxSize != nil && size > *opts.MaxSize {
		return newError(ReasonSize, `validation: "`+key+`" is over the configured size limit`)
	}
	if opts.MinSize != nil && size < *opts.MinSize {
		return newError(ReasonSize, `validation: "`+key+`" is under the configured size minimum`)
	}
	return nil
}

func resolveType(key string, body files.Body, override string) string {
	if override != "" {
		return override
	}
	if body.ContentType() != "" {
		return body.ContentType()
	}
	if ext := filepath.Ext(key); ext != "" {
		if t := mime.TypeByExtension(ext); t != "" {
			return t
		}
	}
	return "application/octet-stream"
}

func typeAllowed(contentType string, allowed []string) bool {
	actual := baseType(contentType)
	slash := strings.Index(actual, "/")
	group := ""
	if slash >= 0 {
		group = actual[:slash+1]
	}
	for _, candidate := range allowed {
		pattern := baseType(candidate)
		if pattern == actual {
			return true
		}
		if strings.HasSuffix(pattern, "/*") && strings.TrimSuffix(pattern, "*") == group {
			return true
		}
	}
	return false
}

func baseType(value string) string {
	if idx := strings.Index(value, ";"); idx >= 0 {
		value = value[:idx]
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func newError(reason Reason, message string) *files.Error {
	return files.NewError(files.ErrProvider, message, &Error{Reason: reason, Message: message})
}
