package appwrite

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	files "github.com/cersho/gofiles-sdk"
	"github.com/cersho/gofiles-sdk/internal/typeutil"
)

const appwritePartSize int64 = 5 * 1024 * 1024

func (a *Adapter) ResumableUpload(_ context.Context, key string, opts files.ResumableUploadOptions) (files.ResumableDriver, error) {
	if len(opts.Metadata) > 0 {
		return nil, files.NewError(files.ErrProvider, "appwrite: resumable uploads do not support metadata", nil)
	}
	if opts.CacheControl != "" {
		return nil, files.NewError(files.ErrProvider, "appwrite: resumable uploads do not support cacheControl", nil)
	}
	if a.key == "" {
		return nil, files.NewError(files.ErrProvider, "appwrite: resumable uploads require an API key. Pass Key or set APPWRITE_API_KEY / APPWRITE_KEY.", nil)
	}
	if err := validateFileID(key, "key"); err != nil {
		return nil, err
	}
	return &resumableDriver{adapter: a, key: key}, nil
}

type resumableDriver struct {
	adapter  *Adapter
	key      string
	session  files.ResumableSession
	lastFile appwriteFile
}

func (d *resumableDriver) Begin(_ context.Context, meta files.ResumableUploadMeta) (files.ResumableSession, error) {
	session := files.ResumableSession{
		Provider:    "appwrite",
		Key:         d.key,
		Bucket:      d.adapter.bucket,
		UploadID:    d.key,
		PartSize:    appwritePartSize,
		ContentType: meta.ContentType,
		TempPath:    strconv.FormatInt(meta.Size, 10),
	}
	d.session = session
	return session, nil
}

func (d *resumableDriver) Adopt(_ context.Context, session files.ResumableSession) error {
	if session.Provider != "appwrite" || session.Key != d.key || session.Bucket != d.adapter.bucket {
		return files.NewError(files.ErrProvider, "appwrite: resumable session does not match this upload", nil)
	}
	if session.PartSize > 0 && session.PartSize != appwritePartSize {
		return files.NewError(files.ErrProvider, "appwrite: resumable session part size must be 5 MiB", nil)
	}
	if session.UploadID != "" && session.UploadID != d.key {
		return files.NewError(files.ErrProvider, "appwrite: resumable session upload ID does not match this upload", nil)
	}
	session.PartSize = appwritePartSize
	session.UploadID = d.key
	d.session = session
	return nil
}

func (d *resumableDriver) Probe(context.Context) (files.ResumableProbe, error) {
	if d.session.Provider == "" {
		return files.ResumableProbe{}, files.NewError(files.ErrProvider, "appwrite: resumable session is not initialized", nil)
	}
	return files.ResumableProbe{NextOffset: sumParts(d.session.Parts), Parts: append([]files.PartMeta(nil), d.session.Parts...)}, nil
}

func (d *resumableDriver) UploadPart(ctx context.Context, part files.ResumablePart) (files.PartMeta, error) {
	if d.session.Provider == "" {
		return files.PartMeta{}, files.NewError(files.ErrProvider, "appwrite: resumable session is not initialized", nil)
	}
	total := d.sessionSizeWith(part)
	end := part.Offset + int64(len(part.Data)) - 1
	headers := http.Header{}
	headers.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", part.Offset, end, total))
	headers.Set("X-Appwrite-ID", d.key)
	var decoded appwriteFile
	if err := d.adapter.uploadReader(ctx, d.key, d.key, d.session.ContentType, readerFromBytes(part.Data), -1, headers, &decoded); err != nil {
		return files.PartMeta{}, err
	}
	d.lastFile = decoded
	meta := files.PartMeta{PartNumber: part.PartNumber, Size: int64(len(part.Data))}
	d.session.Parts = upsertPart(d.session.Parts, meta)
	return meta, nil
}

func (d *resumableDriver) Complete(_ context.Context, parts []files.PartMeta) (files.UploadResult, error) {
	size := sumParts(parts)
	return files.UploadResult{
		Key:         first(d.lastFile.ID, d.key),
		Size:        firstInt64(d.lastFile.SizeOriginal, size),
		ContentType: typeutil.EffectiveContentType(d.lastFile.MimeType, d.session.ContentType),
	}, nil
}

func (d *resumableDriver) Abort(ctx context.Context) error {
	return d.adapter.Delete(ctx, d.key, files.OperationOptions{})
}

func (d *resumableDriver) sessionSizeWith(part files.ResumablePart) int64 {
	if d.session.TempPath != "" {
		if total, err := strconv.ParseInt(d.session.TempPath, 10, 64); err == nil && total > 0 {
			return total
		}
	}
	current := sumParts(d.session.Parts)
	next := part.Offset + int64(len(part.Data))
	if next > current {
		return next
	}
	return current
}

func upsertPart(parts []files.PartMeta, part files.PartMeta) []files.PartMeta {
	for i := range parts {
		if parts[i].PartNumber == part.PartNumber {
			parts[i] = part
			return parts
		}
	}
	return append(parts, part)
}

func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
