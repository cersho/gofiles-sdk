package supabase

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"strconv"

	files "github.com/cersho/gofiles-sdk"
)

const minTusPartSize int64 = 6 * 1024 * 1024

func (a *Adapter) ResumableUpload(_ context.Context, key string, opts files.ResumableUploadOptions) (files.ResumableDriver, error) {
	if len(opts.Metadata) > 0 {
		return nil, files.NewError(files.ErrProvider, "supabase: resumable uploads do not support metadata", nil)
	}
	return &resumableDriver{adapter: a, key: key, opts: opts}, nil
}

type resumableDriver struct {
	adapter *Adapter
	key     string
	opts    files.ResumableUploadOptions
	session files.ResumableSession
}

func (d *resumableDriver) Begin(ctx context.Context, meta files.ResumableUploadMeta) (files.ResumableSession, error) {
	partSize := meta.PartSize
	if partSize < minTusPartSize {
		partSize = minTusPartSize
	}
	headers := d.adapter.tusHeaders()
	headers.Set("Upload-Length", strconv.FormatInt(meta.Size, 10))
	headers.Set("Upload-Metadata", "bucketName "+b64(d.adapter.bucket)+",objectName "+b64(d.key)+",contentType "+b64(meta.ContentType))
	headers.Set("x-upsert", "true")
	res, err := d.adapter.doTus(ctx, http.MethodPost, d.adapter.storageURL+"/upload/resumable", headers, nil)
	if err != nil {
		return files.ResumableSession{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return files.ResumableSession{}, files.NewError(files.ErrProvider, "supabase: resumable session init failed (HTTP "+strconv.Itoa(res.StatusCode)+")", nil)
	}
	location := res.Header.Get("Location")
	if location == "" {
		return files.ResumableSession{}, files.NewError(files.ErrProvider, "supabase: resumable session response missing Location header", nil)
	}
	session := files.ResumableSession{
		Provider:    "supabase",
		Key:         d.key,
		Bucket:      d.adapter.bucket,
		TempPath:    d.adapter.absoluteURL(location),
		PartSize:    partSize,
		ContentType: meta.ContentType,
	}
	d.session = session
	return session, nil
}

func (d *resumableDriver) Adopt(_ context.Context, session files.ResumableSession) error {
	if session.Provider != "supabase" || session.Key != d.key || session.TempPath == "" {
		return files.NewError(files.ErrProvider, "supabase: resumable session does not match this upload", nil)
	}
	if session.PartSize > 0 && session.PartSize < minTusPartSize {
		return files.NewError(files.ErrProvider, "supabase: resumable session part size is below the 6 MiB minimum", nil)
	}
	d.session = session
	return nil
}

func (d *resumableDriver) Probe(ctx context.Context) (files.ResumableProbe, error) {
	if d.session.TempPath == "" {
		return files.ResumableProbe{}, files.NewError(files.ErrProvider, "supabase: resumable session is not initialized", nil)
	}
	res, err := d.adapter.doTus(ctx, http.MethodHead, d.session.TempPath, d.adapter.tusHeaders(), nil)
	if err != nil {
		return files.ResumableProbe{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return files.ResumableProbe{}, files.NewError(files.ErrProvider, "supabase: resume status check failed (HTTP "+strconv.Itoa(res.StatusCode)+")", nil)
	}
	offset, _ := strconv.ParseInt(res.Header.Get("Upload-Offset"), 10, 64)
	return files.ResumableProbe{NextOffset: offset}, nil
}

func (d *resumableDriver) UploadPart(ctx context.Context, part files.ResumablePart) (files.PartMeta, error) {
	if d.session.TempPath == "" {
		return files.PartMeta{}, files.NewError(files.ErrProvider, "supabase: resumable session is not initialized", nil)
	}
	headers := d.adapter.tusHeaders()
	headers.Set("Content-Type", "application/offset+octet-stream")
	headers.Set("Upload-Offset", strconv.FormatInt(part.Offset, 10))
	res, err := d.adapter.doTus(ctx, http.MethodPatch, d.session.TempPath, headers, bytes.NewReader(part.Data))
	if err != nil {
		return files.PartMeta{}, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return files.PartMeta{}, files.NewError(files.ErrProvider, "supabase: chunk upload failed (HTTP "+strconv.Itoa(res.StatusCode)+")", nil)
	}
	return files.PartMeta{PartNumber: part.PartNumber, Size: int64(len(part.Data))}, nil
}

func (d *resumableDriver) Complete(_ context.Context, parts []files.PartMeta) (files.UploadResult, error) {
	var size int64
	for _, part := range parts {
		size += part.Size
	}
	return files.UploadResult{Key: d.key, Size: size, ContentType: d.session.ContentType}, nil
}

func (d *resumableDriver) Abort(ctx context.Context) error {
	if d.session.TempPath == "" {
		return nil
	}
	res, err := d.adapter.doTus(ctx, http.MethodDelete, d.session.TempPath, d.adapter.tusHeaders(), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return nil
}

func (a *Adapter) tusHeaders() http.Header {
	headers := http.Header{}
	headers.Set("apikey", a.key)
	if token := a.authBearer(); token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	headers.Set("Tus-Resumable", "1.0.0")
	return headers
}

func (a *Adapter) doTus(ctx context.Context, method string, rawURL string, headers http.Header, body *bytes.Reader) (*http.Response, error) {
	var reader ioReader
	if body != nil {
		reader = body
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, files.NewError(files.ErrProvider, err.Error(), err)
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	return a.httpClient.Do(req)
}

type ioReader interface {
	Read([]byte) (int, error)
}

func b64(value string) string {
	return base64.StdEncoding.EncodeToString([]byte(value))
}
