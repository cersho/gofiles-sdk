package memory

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/maputil"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/rangeutil"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/typeutil"
)

type Seed struct {
	Body         []byte
	ContentType  string
	Metadata     map[string]string
	CacheControl string
}

type Options struct {
	Initial map[string]Seed
}

type Entry struct {
	Bytes        []byte
	ContentType  string
	Metadata     map[string]string
	CacheControl string
	ETag         string
	LastModified time.Time
}

type Adapter struct {
	mu      sync.RWMutex
	store   map[string]Entry
	pending map[string]*pendingUpload
	seq     int64
}

type pendingUpload struct {
	Chunks      [][]byte
	Received    int64
	ContentType string
	Metadata    map[string]string
	Cache       string
}

func New(opts Options) *Adapter {
	a := &Adapter{store: map[string]Entry{}, pending: map[string]*pendingUpload{}}
	for key, seed := range opts.Initial {
		contentType := seed.ContentType
		if contentType == "" {
			contentType = "application/octet-stream"
		}
		a.put(key, seed.Body, contentType, seed.Metadata, seed.CacheControl)
	}
	return a
}

func (a *Adapter) Name() string { return "memory" }

func (a *Adapter) Raw() any { return a.store }

func (a *Adapter) Capabilities() files.AdapterCapabilities {
	return files.AdapterCapabilities{
		RangeRead:      true,
		Delimiter:      true,
		Metadata:       true,
		CacheControl:   true,
		Multipart:      true,
		Resumable:      true,
		ServerSideCopy: true,
		SignedURL:      files.SignedURLCapability{Supported: false},
	}
}

func (a *Adapter) Upload(ctx context.Context, key string, body files.Body, opts files.UploadOptions) (files.UploadResult, error) {
	data, err := body.ReadAll(ctx)
	if err != nil {
		return files.UploadResult{}, err
	}
	contentType := typeutil.EffectiveContentType(opts.ContentType, body.ContentType())
	entry := a.put(key, data, contentType, opts.Metadata, opts.CacheControl)
	return uploadResult(key, entry), nil
}

func (a *Adapter) Download(_ context.Context, key string, opts files.DownloadOptions) (files.StoredFile, error) {
	entry, ok := a.get(key)
	if !ok {
		return files.StoredFile{}, files.NewError(files.ErrNotFound, "memory: not found: "+key, nil)
	}
	data := append([]byte(nil), entry.Bytes...)
	if opts.Range != nil {
		data = sliceRange(data, *opts.Range)
	}
	return storedFile(key, entry, data), nil
}

func (a *Adapter) Head(_ context.Context, key string, _ files.OperationOptions) (files.StoredFile, error) {
	entry, ok := a.get(key)
	if !ok {
		return files.StoredFile{}, files.NewError(files.ErrNotFound, "memory: not found: "+key, nil)
	}
	return files.NewStoredFile(files.StoredFileMeta{
		Key:          key,
		Size:         int64(len(entry.Bytes)),
		ContentType:  entry.ContentType,
		LastModified: entry.LastModified,
		ETag:         entry.ETag,
		Metadata:     cloneMap(entry.Metadata),
	}, func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(append([]byte(nil), entry.Bytes...))), nil
	}), nil
}

func (a *Adapter) Exists(ctx context.Context, key string, opts files.OperationOptions) (bool, error) {
	_, err := a.Head(ctx, key, opts)
	if err == nil {
		return true, nil
	}
	if files.IsCode(err, files.ErrNotFound) {
		return false, nil
	}
	return false, err
}

func (a *Adapter) Delete(_ context.Context, key string, _ files.OperationOptions) error {
	a.mu.Lock()
	delete(a.store, key)
	a.mu.Unlock()
	return nil
}

func (a *Adapter) Copy(_ context.Context, from string, to string, _ files.OperationOptions) error {
	entry, ok := a.get(from)
	if !ok {
		return files.NewError(files.ErrNotFound, "memory: not found: "+from, nil)
	}
	a.put(to, entry.Bytes, entry.ContentType, entry.Metadata, entry.CacheControl)
	return nil
}

func (a *Adapter) Move(_ context.Context, from string, to string, _ files.OperationOptions) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.store[from]
	if !ok {
		return files.NewError(files.ErrNotFound, "memory: not found: "+from, nil)
	}
	delete(a.store, from)
	a.store[to] = entry
	return nil
}

func (a *Adapter) List(_ context.Context, opts files.ListOptions) (files.ListResult, error) {
	a.mu.RLock()
	keys := make([]string, 0, len(a.store))
	for key := range a.store {
		if opts.Prefix == "" || strings.HasPrefix(key, opts.Prefix) {
			keys = append(keys, key)
		}
	}
	a.mu.RUnlock()
	sort.Strings(keys)
	start := 0
	if opts.Cursor != "" {
		for start < len(keys) && keys[start] <= opts.Cursor {
			start++
		}
	}
	limit := int(opts.Limit)
	if limit <= 0 {
		limit = 1000
	}
	result := files.ListResult{}
	prefixes := map[string]bool{}
	for i := start; i < len(keys) && len(result.Items) < limit; i++ {
		key := keys[i]
		if opts.Delimiter != "" {
			rest := strings.TrimPrefix(key, opts.Prefix)
			if idx := strings.Index(rest, opts.Delimiter); idx >= 0 {
				prefixes[opts.Prefix+rest[:idx+len(opts.Delimiter)]] = true
				continue
			}
		}
		entry, _ := a.get(key)
		result.Items = append(result.Items, files.NewStoredFile(files.StoredFileMeta{
			Key:          key,
			Size:         int64(len(entry.Bytes)),
			ContentType:  entry.ContentType,
			LastModified: entry.LastModified,
			ETag:         entry.ETag,
			Metadata:     cloneMap(entry.Metadata),
		}, func(context.Context) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(append([]byte(nil), entry.Bytes...))), nil
		}))
		if len(result.Items) == limit && i+1 < len(keys) {
			result.Cursor = key
		}
	}
	for prefix := range prefixes {
		result.Prefixes = append(result.Prefixes, prefix)
	}
	sort.Strings(result.Prefixes)
	return result, nil
}

func (a *Adapter) URL(_ context.Context, key string, opts files.URLOptions) (string, error) {
	if _, ok := a.get(key); !ok {
		return "", files.NewError(files.ErrNotFound, "memory: not found: "+key, nil)
	}
	u := "memory://" + url.PathEscape(key)
	q := url.Values{}
	if opts.ExpiresIn > 0 {
		q.Set("expires", opts.ExpiresIn.String())
	}
	if opts.ResponseContentDisposition != "" {
		q.Set("response-content-disposition", opts.ResponseContentDisposition)
	}
	if encoded := q.Encode(); encoded != "" {
		u += "?" + encoded
	}
	return u, nil
}

func (a *Adapter) SignedUploadURL(_ context.Context, key string, opts files.SignedUploadOptions) (files.SignedUpload, error) {
	headers := map[string]string{}
	if opts.ContentType != "" {
		headers["Content-Type"] = opts.ContentType
	}
	return files.SignedUpload{Method: "PUT", URL: "memory://" + url.PathEscape(key), Headers: headers}, nil
}

func (a *Adapter) ResumableUpload(_ context.Context, key string, opts files.ResumableUploadOptions) (files.ResumableDriver, error) {
	return &resumableDriver{adapter: a, key: key, opts: opts}, nil
}

func (a *Adapter) put(key string, data []byte, contentType string, metadata map[string]string, cacheControl string) Entry {
	copyData := append([]byte(nil), data...)
	entry := Entry{
		Bytes:        copyData,
		ContentType:  contentType,
		Metadata:     cloneMap(metadata),
		CacheControl: cacheControl,
		ETag:         fmt.Sprintf("%x", len(copyData)),
		LastModified: time.Now(),
	}
	a.mu.Lock()
	a.store[key] = entry
	a.mu.Unlock()
	return entry
}

func (a *Adapter) get(key string) (Entry, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	entry, ok := a.store[key]
	if !ok {
		return Entry{}, false
	}
	entry.Bytes = append([]byte(nil), entry.Bytes...)
	entry.Metadata = cloneMap(entry.Metadata)
	return entry, true
}

type resumableDriver struct {
	adapter  *Adapter
	key      string
	opts     files.ResumableUploadOptions
	uploadID string
}

func (d *resumableDriver) Begin(_ context.Context, meta files.ResumableUploadMeta) (files.ResumableSession, error) {
	d.adapter.mu.Lock()
	d.adapter.seq++
	d.uploadID = fmt.Sprintf("mem-%d", d.adapter.seq)
	d.adapter.pending[d.uploadID] = &pendingUpload{
		ContentType: meta.ContentType,
		Metadata:    cloneMap(meta.Metadata),
		Cache:       meta.CacheControl,
	}
	d.adapter.mu.Unlock()
	return files.ResumableSession{
		Provider:    "memory",
		Key:         d.key,
		UploadID:    d.uploadID,
		PartSize:    meta.PartSize,
		ContentType: meta.ContentType,
	}, nil
}

func (d *resumableDriver) Adopt(_ context.Context, session files.ResumableSession) error {
	if session.Provider != "memory" || session.Key != d.key {
		return files.NewError(files.ErrProvider, "memory: resumable session does not match this upload", nil)
	}
	d.adapter.mu.RLock()
	_, ok := d.adapter.pending[session.UploadID]
	d.adapter.mu.RUnlock()
	if !ok {
		return files.NewError(files.ErrProvider, "memory: resumable session not found", nil)
	}
	d.uploadID = session.UploadID
	return nil
}

func (d *resumableDriver) Probe(context.Context) (files.ResumableProbe, error) {
	pending, err := d.pending()
	if err != nil {
		return files.ResumableProbe{}, err
	}
	return files.ResumableProbe{NextOffset: pending.Received}, nil
}

func (d *resumableDriver) UploadPart(_ context.Context, part files.ResumablePart) (files.PartMeta, error) {
	pending, err := d.pending()
	if err != nil {
		return files.PartMeta{}, err
	}
	d.adapter.mu.Lock()
	pending.Chunks = append(pending.Chunks, append([]byte(nil), part.Data...))
	pending.Received = part.Offset + int64(len(part.Data))
	d.adapter.mu.Unlock()
	return files.PartMeta{PartNumber: part.PartNumber, Size: int64(len(part.Data))}, nil
}

func (d *resumableDriver) Complete(context.Context, []files.PartMeta) (files.UploadResult, error) {
	pending, err := d.pending()
	if err != nil {
		return files.UploadResult{}, err
	}
	total := int(pending.Received)
	data := make([]byte, 0, total)
	for _, chunk := range pending.Chunks {
		data = append(data, chunk...)
	}
	entry := d.adapter.put(d.key, data, pending.ContentType, pending.Metadata, pending.Cache)
	d.adapter.mu.Lock()
	delete(d.adapter.pending, d.uploadID)
	d.adapter.mu.Unlock()
	return uploadResult(d.key, entry), nil
}

func (d *resumableDriver) Abort(context.Context) error {
	d.adapter.mu.Lock()
	delete(d.adapter.pending, d.uploadID)
	d.adapter.mu.Unlock()
	return nil
}

func (d *resumableDriver) pending() (*pendingUpload, error) {
	d.adapter.mu.RLock()
	defer d.adapter.mu.RUnlock()
	pending, ok := d.adapter.pending[d.uploadID]
	if !ok {
		return nil, files.NewError(files.ErrProvider, "memory: resumable session not found", nil)
	}
	return pending, nil
}

func storedFile(key string, entry Entry, data []byte) files.StoredFile {
	return files.NewStoredFile(files.StoredFileMeta{
		Key:          key,
		Size:         int64(len(data)),
		ContentType:  entry.ContentType,
		LastModified: entry.LastModified,
		ETag:         entry.ETag,
		Metadata:     cloneMap(entry.Metadata),
	}, func(context.Context) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(data)), nil
	})
}

func uploadResult(key string, entry Entry) files.UploadResult {
	return files.UploadResult{
		Key:          key,
		Size:         int64(len(entry.Bytes)),
		ContentType:  entry.ContentType,
		ETag:         entry.ETag,
		LastModified: entry.LastModified,
	}
}

func sliceRange(data []byte, r files.ByteRange) []byte {
	return rangeutil.Slice(data, r.Start, r.End)
}

func cloneMap(in map[string]string) map[string]string {
	return maputil.CloneStringMap(in)
}
