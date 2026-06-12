package vercelblob

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	files "github.com/cersho/gofiles-sdk"
	"github.com/cersho/gofiles-sdk/internal/typeutil"
)

const minMultipartPartSize int64 = 5 * 1024 * 1024

type createMultipartResponse struct {
	Key      string `json:"key"`
	UploadID string `json:"uploadId"`
}

type uploadPartResponse struct {
	ETag string `json:"etag"`
}

type completedPart struct {
	ETag       string `json:"etag"`
	PartNumber int    `json:"partNumber"`
}

func (a *Adapter) ResumableUpload(_ context.Context, key string, opts files.ResumableUploadOptions) (files.ResumableDriver, error) {
	return &resumableDriver{adapter: a, key: key, opts: opts}, nil
}

type resumableDriver struct {
	adapter *Adapter
	key     string
	opts    files.ResumableUploadOptions
	session files.ResumableSession
}

func (d *resumableDriver) Begin(ctx context.Context, meta files.ResumableUploadMeta) (files.ResumableSession, error) {
	if err := validatePathname(d.key); err != nil {
		return files.ResumableSession{}, err
	}
	partSize := meta.PartSize
	if partSize < minMultipartPartSize {
		partSize = minMultipartPartSize
	}
	headers := d.adapter.createBlobHeaders(meta.ContentType, meta.CacheControl)
	headers.Set("x-mpu-action", "create")
	var decoded createMultipartResponse
	if _, err := d.adapter.requestAPI(ctx, http.MethodPost, "/mpu"+queryPath(url.Values{"pathname": []string{d.key}}), headers, nil, -1, &decoded); err != nil {
		return files.ResumableSession{}, err
	}
	session := files.ResumableSession{
		Provider:    "vercel-blob",
		Key:         d.key,
		Bucket:      d.adapter.auth.storeID,
		UploadID:    decoded.UploadID,
		TempPath:    decoded.Key,
		PartSize:    partSize,
		ContentType: meta.ContentType,
	}
	d.session = session
	return session, nil
}

func (d *resumableDriver) Adopt(_ context.Context, session files.ResumableSession) error {
	if session.Provider != "vercel-blob" || session.Key != d.key || session.UploadID == "" || session.TempPath == "" {
		return files.NewError(files.ErrProvider, "vercel-blob: resumable session does not match this upload", nil)
	}
	if session.PartSize > 0 && session.PartSize < minMultipartPartSize {
		return files.NewError(files.ErrProvider, "vercel-blob: resumable session part size is below the 5 MiB minimum", nil)
	}
	d.session = session
	return nil
}

func (d *resumableDriver) Probe(context.Context) (files.ResumableProbe, error) {
	if d.session.UploadID == "" {
		return files.ResumableProbe{}, files.NewError(files.ErrProvider, "vercel-blob: resumable session is not initialized", nil)
	}
	parts := append([]files.PartMeta(nil), d.session.Parts...)
	sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })
	var offset int64
	for _, part := range parts {
		offset += part.Size
	}
	return files.ResumableProbe{NextOffset: offset, Parts: parts}, nil
}

func (d *resumableDriver) UploadPart(ctx context.Context, part files.ResumablePart) (files.PartMeta, error) {
	if d.session.UploadID == "" {
		return files.PartMeta{}, files.NewError(files.ErrProvider, "vercel-blob: resumable session is not initialized", nil)
	}
	headers := d.adapter.createBlobHeaders(d.session.ContentType, d.opts.CacheControl)
	headers.Set("x-mpu-action", "upload")
	headers.Set("x-mpu-key", url.PathEscape(d.session.TempPath))
	headers.Set("x-mpu-upload-id", d.session.UploadID)
	headers.Set("x-mpu-part-number", strconv.Itoa(part.PartNumber))
	var decoded uploadPartResponse
	if _, err := d.adapter.requestAPI(ctx, http.MethodPost, "/mpu"+queryPath(url.Values{"pathname": []string{d.key}}), headers, bytes.NewReader(part.Data), int64(len(part.Data)), &decoded); err != nil {
		return files.PartMeta{}, err
	}
	meta := files.PartMeta{PartNumber: part.PartNumber, Size: int64(len(part.Data)), ETag: decoded.ETag}
	d.session.Parts = upsertSessionPart(d.session.Parts, meta)
	return meta, nil
}

func (d *resumableDriver) Complete(ctx context.Context, parts []files.PartMeta) (files.UploadResult, error) {
	if d.session.UploadID == "" {
		return files.UploadResult{}, files.NewError(files.ErrProvider, "vercel-blob: resumable session is not initialized", nil)
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })
	completed := make([]completedPart, 0, len(parts))
	var size int64
	for _, part := range parts {
		completed = append(completed, completedPart{ETag: part.ETag, PartNumber: part.PartNumber})
		size += part.Size
	}
	payload, err := json.Marshal(completed)
	if err != nil {
		return files.UploadResult{}, files.NewError(files.ErrProvider, err.Error(), err)
	}
	headers := d.adapter.createBlobHeaders(d.session.ContentType, d.opts.CacheControl)
	headers.Set("Content-Type", "application/json")
	headers.Set("x-mpu-action", "complete")
	headers.Set("x-mpu-key", url.PathEscape(d.session.TempPath))
	headers.Set("x-mpu-upload-id", d.session.UploadID)
	var decoded putBlobResponse
	if _, err := d.adapter.requestAPI(ctx, http.MethodPost, "/mpu"+queryPath(url.Values{"pathname": []string{d.key}}), headers, bytes.NewReader(payload), int64(len(payload)), &decoded); err != nil {
		return files.UploadResult{}, err
	}
	return files.UploadResult{
		Key:          first(decoded.Pathname, d.key),
		Size:         size,
		ContentType:  typeutil.EffectiveContentType(decoded.ContentType, d.session.ContentType),
		ETag:         decoded.ETag,
		LastModified: time.Now(),
	}, nil
}

func (d *resumableDriver) Abort(context.Context) error {
	return nil
}

func upsertSessionPart(parts []files.PartMeta, part files.PartMeta) []files.PartMeta {
	for i := range parts {
		if parts[i].PartNumber == part.PartNumber {
			parts[i] = part
			return parts
		}
	}
	return append(parts, part)
}
