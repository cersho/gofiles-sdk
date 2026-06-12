package fs

import (
	"context"
	"errors"
	"io"
	"os"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/maputil"
)

type limitedReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r limitedReadCloser) Close() error { return r.closer.Close() }

func copyContext(ctx context.Context, dst io.Writer, src io.Reader) (int64, error) {
	buf := make([]byte, 32*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		n, readErr := src.Read(buf)
		if n > 0 {
			m, writeErr := dst.Write(buf[:n])
			written += int64(m)
			if writeErr != nil {
				return written, writeErr
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return written, nil
			}
			return written, readErr
		}
	}
}

func mapFSError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return files.NewError(files.ErrNotFound, err.Error(), err)
	}
	if errors.Is(err, os.ErrPermission) {
		return files.NewError(files.ErrUnauthorized, err.Error(), err)
	}
	return files.NewError(files.ErrProvider, err.Error(), err)
}

func cloneMap(in map[string]string) map[string]string {
	return maputil.CloneStringMap(in)
}
