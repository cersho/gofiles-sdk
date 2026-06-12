package contenttype

import (
	"bytes"
	"context"
	"io"
	"mime"
	"path/filepath"
	"strings"
	"sync"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
)

type OnMismatch string
type OnUnknown string

const (
	OnMismatchCorrect OnMismatch = "correct"
	OnMismatchReject  OnMismatch = "reject"
	OnUnknownTrust    OnUnknown  = "trust"
	OnUnknownReject   OnUnknown  = "reject"
)

type Options struct {
	OnMismatch OnMismatch
	OnUnknown  OnUnknown
}

const sniffBytes = 512
const generic = "application/octet-stream"

func New(opts Options) files.Middleware {
	onMismatch := opts.OnMismatch
	if onMismatch == "" {
		onMismatch = OnMismatchCorrect
	}
	onUnknown := opts.OnUnknown
	if onUnknown == "" {
		onUnknown = OnUnknownTrust
	}
	return func(ctx context.Context, op files.Operation, next files.Handler) (any, error) {
		switch op.Kind {
		case files.OperationSignedUploadURL:
			return nil, files.NewError(files.ErrProvider, "contenttype: signed upload URLs bypass content sniffing; upload through the Files client to enforce it", nil)
		case files.OperationUpload:
			head, body, err := peekBody(ctx, op.Body)
			if err != nil {
				return nil, err
			}
			sniffed := Detect(head)
			if sniffed == "" {
				if onUnknown == OnUnknownReject {
					return nil, files.NewError(files.ErrProvider, `contenttype: could not identify "`+op.Key+`" from its signature`, nil)
				}
				nextOp := op
				nextOp.Body = body
				return next(ctx, nextOp)
			}
			declared := baseType(declaredType(op.Key, op.Body, op.UploadOptions.ContentType))
			if declared == sniffed {
				nextOp := op
				nextOp.Body = body
				return next(ctx, nextOp)
			}
			if onMismatch == OnMismatchReject && declared != generic {
				return nil, files.NewError(files.ErrProvider, `contenttype: "`+op.Key+`" is declared "`+declared+`" but its bytes are "`+sniffed+`"`, nil)
			}
			nextOp := op
			nextOp.Body = body
			nextOp.UploadOptions.ContentType = sniffed
			return next(ctx, nextOp)
		default:
			return next(ctx, op)
		}
	}
}

func Detect(data []byte) string {
	signatures := []struct {
		contentType string
		offsets     []signature
	}{
		{"image/png", []signature{{0, []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}}}},
		{"image/jpeg", []signature{{0, []byte{0xff, 0xd8, 0xff}}}},
		{"image/gif", []signature{{0, []byte("GIF8")}}},
		{"image/bmp", []signature{{0, []byte("BM")}}},
		{"image/webp", []signature{{0, []byte("RIFF")}, {8, []byte("WEBP")}}},
		{"image/tiff", []signature{{0, []byte{0x49, 0x49, 0x2a, 0x00}}}},
		{"image/tiff", []signature{{0, []byte{0x4d, 0x4d, 0x00, 0x2a}}}},
		{"image/x-icon", []signature{{0, []byte{0x00, 0x00, 0x01, 0x00}}}},
		{"application/pdf", []signature{{0, []byte("%PDF")}}},
	}
	for _, item := range signatures {
		ok := true
		for _, sig := range item.offsets {
			if !matchAt(data, sig.offset, sig.bytes) {
				ok = false
				break
			}
		}
		if ok {
			return item.contentType
		}
	}
	return sniffText(data)
}

type signature struct {
	offset int
	bytes  []byte
}

func matchAt(data []byte, offset int, sig []byte) bool {
	if offset+len(sig) > len(data) {
		return false
	}
	return bytes.Equal(data[offset:offset+len(sig)], sig)
}

func sniffText(data []byte) string {
	i := 0
	if matchAt(data, 0, []byte{0xef, 0xbb, 0xbf}) {
		i = 3
	}
	for i < len(data) {
		switch data[i] {
		case '\t', '\n', '\r', '\f', ' ':
			i++
		default:
			goto done
		}
	}
done:
	head := strings.ToLower(string(data[i:min(len(data), i+sniffBytes)]))
	if strings.HasPrefix(head, "<!doctype html") || strings.HasPrefix(head, "<!--") {
		return "text/html"
	}
	for _, tag := range []string{"html", "head", "body", "script", "iframe", "title", "table", "div", "a"} {
		if opensTag(head, tag) {
			return "text/html"
		}
	}
	if opensTag(head, "svg") {
		return "image/svg+xml"
	}
	if strings.HasPrefix(head, "<?xml") {
		if strings.Contains(head, "<svg") {
			return "image/svg+xml"
		}
		return "application/xml"
	}
	return ""
}

func opensTag(head string, tag string) bool {
	prefix := "<" + tag
	if !strings.HasPrefix(head, prefix) {
		return false
	}
	if len(head) == len(prefix) {
		return true
	}
	switch head[len(prefix)] {
	case ' ', '\t', '\n', '\r', '\f', '>', '/':
		return true
	default:
		return false
	}
}

func peekBody(ctx context.Context, body files.Body) ([]byte, files.Body, error) {
	if body.Replayable() {
		reader, err := body.Open(ctx)
		if err != nil {
			return nil, files.Body{}, err
		}
		defer reader.Close()
		head, err := readHead(reader)
		if err != nil {
			return nil, files.Body{}, err
		}
		return head, body, nil
	}
	reader, err := body.Open(ctx)
	if err != nil {
		return nil, files.Body{}, err
	}
	head, err := readHead(reader)
	if err != nil {
		_ = reader.Close()
		return nil, files.Body{}, err
	}
	var mu sync.Mutex
	used := false
	wrapped := files.NewBodyFromReadCloser(func(context.Context) (io.ReadCloser, error) {
		mu.Lock()
		defer mu.Unlock()
		if used {
			return nil, files.NewError(files.ErrProvider, "contenttype: non-replayable body has already been consumed", nil)
		}
		used = true
		return prefixReadCloser{Reader: io.MultiReader(bytes.NewReader(head), reader), closer: reader}, nil
	}, 0, false, body.ContentType(), false)
	return head, wrapped, nil
}

func readHead(reader io.Reader) ([]byte, error) {
	buf := make([]byte, sniffBytes)
	n, err := io.ReadFull(reader, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	return append([]byte(nil), buf[:n]...), nil
}

type prefixReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r prefixReadCloser) Close() error { return r.closer.Close() }

func declaredType(key string, body files.Body, override string) string {
	if override != "" {
		return override
	}
	if body.ContentType() != "" {
		return body.ContentType()
	}
	if t := mime.TypeByExtension(filepath.Ext(key)); t != "" {
		return t
	}
	return generic
}

func baseType(value string) string {
	if idx := strings.Index(value, ";"); idx >= 0 {
		value = value[:idx]
	}
	return strings.ToLower(strings.TrimSpace(value))
}
