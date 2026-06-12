package files

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"sync"
)

type Body struct {
	open        func(context.Context) (io.ReadCloser, error)
	size        int64
	sizeKnown   bool
	contentType string
	replayable  bool
}

func BytesBody(data []byte) Body {
	copied := append([]byte(nil), data...)
	return Body{
		open: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(copied)), nil
		},
		size:       int64(len(copied)),
		sizeKnown:  true,
		replayable: true,
	}
}

func StringBody(value string) Body {
	return Body{
		open: func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader(value)), nil
		},
		size:        int64(len([]byte(value))),
		sizeKnown:   true,
		contentType: "text/plain; charset=utf-8",
		replayable:  true,
	}
}

func ReaderBody(reader io.Reader) Body {
	var mu sync.Mutex
	used := false
	return Body{
		open: func(context.Context) (io.ReadCloser, error) {
			mu.Lock()
			defer mu.Unlock()
			if used {
				return nil, NewError(ErrProvider, "body reader has already been consumed", nil)
			}
			used = true
			if rc, ok := reader.(io.ReadCloser); ok {
				return rc, nil
			}
			return io.NopCloser(reader), nil
		},
		replayable: false,
	}
}

func FileBody(path string) Body {
	info, err := os.Stat(path)
	sizeKnown := err == nil && !info.IsDir()
	size := int64(0)
	if sizeKnown {
		size = info.Size()
	}
	return Body{
		open: func(context.Context) (io.ReadCloser, error) {
			return os.Open(path)
		},
		size:       size,
		sizeKnown:  sizeKnown,
		replayable: true,
	}
}

func (b Body) Open(ctx context.Context) (io.ReadCloser, error) {
	if b.open == nil {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	return b.open(ctx)
}

func (b Body) Size() (int64, bool) {
	return b.size, b.sizeKnown
}

func (b Body) ContentType() string {
	return b.contentType
}

func (b Body) Replayable() bool {
	return b.replayable
}

func (b Body) ReadAll(ctx context.Context) ([]byte, error) {
	r, err := b.Open(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func NewBodyFromReadCloser(open func(context.Context) (io.ReadCloser, error), size int64, sizeKnown bool, contentType string, replayable bool) Body {
	return Body{
		open:        open,
		size:        size,
		sizeKnown:   sizeKnown,
		contentType: contentType,
		replayable:  replayable,
	}
}

type progressReadCloser struct {
	reader io.ReadCloser
	loaded int64
	total  int64
	known  bool
	report func(UploadProgress)
}

func (r *progressReadCloser) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.loaded += int64(n)
		if r.report != nil {
			r.report(UploadProgress{Loaded: r.loaded, Total: r.total, Known: r.known})
		}
	}
	return n, err
}

func (r *progressReadCloser) Close() error {
	return r.reader.Close()
}

func BodyWithProgress(body Body, report func(UploadProgress)) Body {
	size, known := body.Size()
	return Body{
		open: func(ctx context.Context) (io.ReadCloser, error) {
			r, err := body.Open(ctx)
			if err != nil {
				return nil, err
			}
			return &progressReadCloser{
				reader: r,
				total:  size,
				known:  known,
				report: report,
			}, nil
		},
		size:        size,
		sizeKnown:   known,
		contentType: body.ContentType(),
		replayable:  body.Replayable(),
	}
}
