package fs

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"time"

	files "github.com/cersho/gofiles-sdk"
	"github.com/cersho/gofiles-sdk/internal/typeutil"
)

func (a *Adapter) ResumableUpload(_ context.Context, key string, opts files.ResumableUploadOptions) (files.ResumableDriver, error) {
	target, err := a.resolve(key)
	if err != nil {
		return nil, err
	}
	return &resumableDriver{adapter: a, key: key, target: target, opts: opts}, nil
}

type resumableDriver struct {
	adapter *Adapter
	key     string
	target  string
	opts    files.ResumableUploadOptions
	session files.ResumableSession
}

func (d *resumableDriver) Begin(_ context.Context, meta files.ResumableUploadMeta) (files.ResumableSession, error) {
	if err := os.MkdirAll(filepath.Dir(d.target), 0o755); err != nil {
		return files.ResumableSession{}, mapFSError(err)
	}
	session := files.ResumableSession{
		Provider:    "fs",
		Key:         d.key,
		UploadID:    d.target + resumableSuffix,
		TempPath:    d.target + resumableSuffix,
		PartSize:    meta.PartSize,
		ContentType: meta.ContentType,
	}
	d.session = session
	return session, nil
}

func (d *resumableDriver) Adopt(_ context.Context, session files.ResumableSession) error {
	if session.Provider != "fs" || session.Key != d.key {
		return files.NewError(files.ErrProvider, "fs: resumable session does not match this upload", nil)
	}
	d.session = session
	return nil
}

func (d *resumableDriver) Probe(context.Context) (files.ResumableProbe, error) {
	if d.session.TempPath == "" {
		return files.ResumableProbe{}, files.NewError(files.ErrProvider, "fs: resumable session is not initialized", nil)
	}
	info, err := os.Stat(d.session.TempPath)
	if errors.Is(err, os.ErrNotExist) {
		return files.ResumableProbe{}, nil
	}
	if err != nil {
		return files.ResumableProbe{}, mapFSError(err)
	}
	return files.ResumableProbe{NextOffset: info.Size()}, nil
}

func (d *resumableDriver) UploadPart(_ context.Context, part files.ResumablePart) (files.PartMeta, error) {
	f, err := os.OpenFile(d.session.TempPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return files.PartMeta{}, mapFSError(err)
	}
	defer f.Close()
	if _, err := f.WriteAt(part.Data, part.Offset); err != nil {
		return files.PartMeta{}, mapFSError(err)
	}
	return files.PartMeta{PartNumber: part.PartNumber, Size: int64(len(part.Data))}, nil
}

func (d *resumableDriver) Complete(context.Context, []files.PartMeta) (files.UploadResult, error) {
	if err := os.Rename(d.session.TempPath, d.target); err != nil {
		return files.UploadResult{}, mapFSError(err)
	}
	data, err := os.ReadFile(d.target)
	if err != nil {
		return files.UploadResult{}, mapFSError(err)
	}
	hash := sha1.Sum(data)
	contentType := typeutil.EffectiveContentType(d.session.ContentType)
	meta := sidecar{
		ContentType:  contentType,
		Metadata:     cloneMap(d.opts.Metadata),
		CacheControl: d.opts.CacheControl,
		ETag:         hex.EncodeToString(hash[:]),
		LastModified: time.Now(),
	}
	if err := writeSidecar(d.target, meta); err != nil {
		return files.UploadResult{}, err
	}
	return files.UploadResult{Key: d.key, Size: int64(len(data)), ContentType: contentType, ETag: meta.ETag, LastModified: meta.LastModified}, nil
}

func (d *resumableDriver) Abort(context.Context) error {
	if d.session.TempPath != "" {
		_ = os.Remove(d.session.TempPath)
	}
	return nil
}
