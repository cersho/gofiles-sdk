package files

import (
	"context"
	"io"
	"time"
)

type StoredFile struct {
	Key          string
	Name         string
	Size         int64
	ContentType  string
	LastModified time.Time
	ETag         string
	Metadata     map[string]string
	open         func(context.Context) (io.ReadCloser, error)
}

type StoredFileMeta struct {
	Key          string
	Size         int64
	ContentType  string
	LastModified time.Time
	ETag         string
	Metadata     map[string]string
}

func NewStoredFile(meta StoredFileMeta, open func(context.Context) (io.ReadCloser, error)) StoredFile {
	name := meta.Key
	if name == "" {
		name = meta.Key
	}
	contentType := meta.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return StoredFile{
		Key:          meta.Key,
		Name:         name,
		Size:         meta.Size,
		ContentType:  contentType,
		LastModified: meta.LastModified,
		ETag:         meta.ETag,
		Metadata:     cloneStringMap(meta.Metadata),
		open:         open,
	}
}

func NewStoredFileFromBytes(meta StoredFileMeta, data []byte) StoredFile {
	body := BytesBody(data)
	if meta.Size == 0 && len(data) > 0 {
		meta.Size = int64(len(data))
	}
	return NewStoredFile(meta, body.Open)
}

func (f StoredFile) Open(ctx context.Context) (io.ReadCloser, error) {
	if f.open == nil {
		return BytesBody(nil).Open(ctx)
	}
	return f.open(ctx)
}

func (f StoredFile) Bytes(ctx context.Context) ([]byte, error) {
	r, err := f.Open(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func (f StoredFile) Text(ctx context.Context) (string, error) {
	data, err := f.Bytes(ctx)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
