package files

import (
	"context"
	"errors"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryObject struct {
	data        []byte
	contentType string
	metadata    map[string]string
}

type memoryAdapter struct {
	mu             sync.Mutex
	objects        map[string]memoryObject
	failUploads    int
	uploadAttempts int
	lastURLKey     string
}

func newMemoryAdapter() *memoryAdapter {
	return &memoryAdapter{objects: map[string]memoryObject{}}
}

func (a *memoryAdapter) Name() string { return "memory" }

func (a *memoryAdapter) Raw() any { return a.objects }

func (a *memoryAdapter) Capabilities() AdapterCapabilities {
	return AdapterCapabilities{
		RangeRead:      true,
		Delimiter:      true,
		Metadata:       true,
		CacheControl:   true,
		ServerSideCopy: true,
		SignedURL:      SignedURLCapability{Supported: true},
	}
}

func (a *memoryAdapter) Upload(ctx context.Context, key string, body Body, opts UploadOptions) (UploadResult, error) {
	a.mu.Lock()
	a.uploadAttempts++
	if a.failUploads > 0 {
		a.failUploads--
		a.mu.Unlock()
		return UploadResult{}, errors.New("temporary upload failure")
	}
	a.mu.Unlock()

	data, err := body.ReadAll(ctx)
	if err != nil {
		return UploadResult{}, err
	}
	contentType := effectiveContentType(body, opts.ContentType)

	a.mu.Lock()
	a.objects[key] = memoryObject{
		data:        append([]byte(nil), data...),
		contentType: contentType,
		metadata:    cloneStringMap(opts.Metadata),
	}
	a.mu.Unlock()

	return UploadResult{
		Key:          key,
		Size:         int64(len(data)),
		ContentType:  contentType,
		ETag:         "memory-etag",
		LastModified: time.Unix(1, 0),
	}, nil
}

func (a *memoryAdapter) Download(_ context.Context, key string, opts DownloadOptions) (StoredFile, error) {
	a.mu.Lock()
	obj, ok := a.objects[key]
	a.mu.Unlock()
	if !ok {
		return StoredFile{}, NewError(ErrNotFound, "not found", nil)
	}
	data := append([]byte(nil), obj.data...)
	if opts.Range != nil {
		end := int64(len(data)) - 1
		if opts.Range.End != nil && *opts.Range.End < end {
			end = *opts.Range.End
		}
		if opts.Range.Start > end {
			data = nil
		} else {
			data = data[opts.Range.Start : end+1]
		}
	}
	return NewStoredFileFromBytes(StoredFileMeta{
		Key:          key,
		Size:         int64(len(data)),
		ContentType:  obj.contentType,
		LastModified: time.Unix(1, 0),
		ETag:         "memory-etag",
		Metadata:     obj.metadata,
	}, data), nil
}

func (a *memoryAdapter) Head(_ context.Context, key string, _ OperationOptions) (StoredFile, error) {
	a.mu.Lock()
	obj, ok := a.objects[key]
	a.mu.Unlock()
	if !ok {
		return StoredFile{}, NewError(ErrNotFound, "not found", nil)
	}
	return NewStoredFile(StoredFileMeta{
		Key:          key,
		Size:         int64(len(obj.data)),
		ContentType:  obj.contentType,
		LastModified: time.Unix(1, 0),
		ETag:         "memory-etag",
		Metadata:     obj.metadata,
	}, nil), nil
}

func (a *memoryAdapter) Exists(ctx context.Context, key string, opts OperationOptions) (bool, error) {
	_, err := a.Head(ctx, key, opts)
	if err == nil {
		return true, nil
	}
	if IsCode(err, ErrNotFound) {
		return false, nil
	}
	return false, err
}

func (a *memoryAdapter) Delete(_ context.Context, key string, _ OperationOptions) error {
	a.mu.Lock()
	delete(a.objects, key)
	a.mu.Unlock()
	return nil
}

func (a *memoryAdapter) Copy(_ context.Context, from string, to string, _ OperationOptions) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	obj, ok := a.objects[from]
	if !ok {
		return NewError(ErrNotFound, "not found", nil)
	}
	a.objects[to] = memoryObject{
		data:        append([]byte(nil), obj.data...),
		contentType: obj.contentType,
		metadata:    cloneStringMap(obj.metadata),
	}
	return nil
}

func (a *memoryAdapter) List(_ context.Context, opts ListOptions) (ListResult, error) {
	a.mu.Lock()
	keys := make([]string, 0, len(a.objects))
	for key := range a.objects {
		keys = append(keys, key)
	}
	a.mu.Unlock()
	sort.Strings(keys)

	offset := 0
	if opts.Cursor != "" {
		parsed, err := strconv.Atoi(opts.Cursor)
		if err != nil {
			return ListResult{}, err
		}
		offset = parsed
	}

	limit := len(keys)
	if opts.Limit > 0 {
		limit = int(opts.Limit)
	}
	result := ListResult{}
	prefixes := map[string]bool{}
	seen := 0
	for _, key := range keys {
		if opts.Prefix != "" && !strings.HasPrefix(key, opts.Prefix) {
			continue
		}
		if seen < offset {
			seen++
			continue
		}
		if len(result.Items) >= limit {
			result.Cursor = strconv.Itoa(seen)
			break
		}
		if opts.Delimiter != "" {
			rest := strings.TrimPrefix(key, opts.Prefix)
			if idx := strings.Index(rest, opts.Delimiter); idx >= 0 {
				prefixes[opts.Prefix+rest[:idx+len(opts.Delimiter)]] = true
				seen++
				continue
			}
		}
		file, err := a.Head(context.Background(), key, OperationOptions{})
		if err != nil {
			return ListResult{}, err
		}
		result.Items = append(result.Items, file)
		seen++
	}
	for prefix := range prefixes {
		result.Prefixes = append(result.Prefixes, prefix)
	}
	sort.Strings(result.Prefixes)
	return result, nil
}

func (a *memoryAdapter) URL(_ context.Context, key string, _ URLOptions) (string, error) {
	a.mu.Lock()
	a.lastURLKey = key
	a.mu.Unlock()
	return "memory://" + url.PathEscape(key), nil
}

func (a *memoryAdapter) SignedUploadURL(_ context.Context, key string, _ SignedUploadOptions) (SignedUpload, error) {
	return SignedUpload{Method: "PUT", URL: "memory://" + url.PathEscape(key)}, nil
}

func TestClientAppliesPrefixAndStripsResults(t *testing.T) {
	ctx := context.Background()
	adapter := newMemoryAdapter()
	client, err := New(Options{Adapter: adapter, Prefix: "/tenant-a/"})
	if err != nil {
		t.Fatal(err)
	}

	out, err := client.Upload(ctx, "avatars/me.txt", StringBody("hello"), UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Key != "avatars/me.txt" {
		t.Fatalf("upload result key = %q", out.Key)
	}
	if _, ok := adapter.objects["tenant-a/avatars/me.txt"]; !ok {
		t.Fatalf("adapter did not receive prefixed key")
	}

	file, err := client.Download(ctx, "avatars/me.txt", DownloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if file.Key != "avatars/me.txt" {
		t.Fatalf("download key = %q", file.Key)
	}
	text, err := file.Text(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello" {
		t.Fatalf("download text = %q", text)
	}

	list, err := client.List(ctx, ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Key != "avatars/me.txt" {
		t.Fatalf("list items = %#v", list.Items)
	}

	if _, err := client.URL(ctx, "avatars/me.txt", URLOptions{}); err != nil {
		t.Fatal(err)
	}
	if adapter.lastURLKey != "tenant-a/avatars/me.txt" {
		t.Fatalf("url key = %q", adapter.lastURLKey)
	}
}

func TestReadOnlyRejectsWritesAndAllowsReads(t *testing.T) {
	ctx := context.Background()
	adapter := newMemoryAdapter()
	client := MustNew(Options{Adapter: adapter})
	if _, err := client.Upload(ctx, "a.txt", StringBody("a"), UploadOptions{}); err != nil {
		t.Fatal(err)
	}

	readonly := client.ReadOnly()
	if err := readonly.Delete(ctx, "a.txt", OperationOptions{}); !IsCode(err, ErrReadOnly) {
		t.Fatalf("delete error = %#v", err)
	}
	if _, err := readonly.Upload(ctx, "b.txt", StringBody("b"), UploadOptions{}); !IsCode(err, ErrReadOnly) {
		t.Fatalf("upload error = %#v", err)
	}
	if _, err := readonly.Download(ctx, "a.txt", DownloadOptions{}); err != nil {
		t.Fatalf("readonly download failed: %v", err)
	}
}

func TestMiddlewareCanInterceptOperation(t *testing.T) {
	ctx := context.Background()
	adapter := newMemoryAdapter()
	client := MustNew(Options{
		Adapter: adapter,
		Middleware: []Middleware{
			Handlers(map[OperationKind]Middleware{
				OperationUpload: func(_ context.Context, op Operation, _ Handler) (any, error) {
					return UploadResult{Key: op.Key, Size: 42, ContentType: "text/plain"}, nil
				},
			}),
		},
	})

	out, err := client.Upload(ctx, "a.txt", StringBody("ignored"), UploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if out.Size != 42 {
		t.Fatalf("intercepted size = %d", out.Size)
	}
	if len(adapter.objects) != 0 {
		t.Fatalf("adapter should not have been called")
	}
}

func TestRetryOnlyRetriesReplayableUploads(t *testing.T) {
	ctx := context.Background()
	adapter := newMemoryAdapter()
	adapter.failUploads = 1
	client := MustNew(Options{
		Adapter: adapter,
		Retries: &RetryOptions{Max: 1, Backoff: func(RetryBackoffContext) time.Duration { return 0 }},
	})

	if _, err := client.Upload(ctx, "a.txt", BytesBody([]byte("a")), UploadOptions{}); err != nil {
		t.Fatal(err)
	}
	if adapter.uploadAttempts != 2 {
		t.Fatalf("replayable upload attempts = %d", adapter.uploadAttempts)
	}

	adapter = newMemoryAdapter()
	adapter.failUploads = 1
	client = MustNew(Options{
		Adapter: adapter,
		Retries: &RetryOptions{Max: 3, Backoff: func(RetryBackoffContext) time.Duration { return 0 }},
	})
	if _, err := client.Upload(ctx, "b.txt", ReaderBody(strings.NewReader("b")), UploadOptions{}); err == nil {
		t.Fatal("expected non-replayable upload to fail")
	}
	if adapter.uploadAttempts != 1 {
		t.Fatalf("non-replayable upload attempts = %d", adapter.uploadAttempts)
	}
}

func TestSearchUsesListAllPagination(t *testing.T) {
	ctx := context.Background()
	adapter := newMemoryAdapter()
	client := MustNew(Options{Adapter: adapter})
	for _, key := range []string{"docs/a.txt", "photos/a.jpg", "photos/b.png"} {
		if _, err := client.Upload(ctx, key, StringBody(key), UploadOptions{}); err != nil {
			t.Fatal(err)
		}
	}

	var found []string
	err := client.Search(ctx, "photos/*.jpg", SearchOptions{Limit: 1}, func(file StoredFile) error {
		found = append(found, file.Key)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0] != "photos/a.jpg" {
		t.Fatalf("found = %#v", found)
	}
}

func TestBodyWithProgressReportsReadBytes(t *testing.T) {
	ctx := context.Background()
	var progress []UploadProgress
	body := BodyWithProgress(StringBody("hello"), func(p UploadProgress) {
		progress = append(progress, p)
	})
	reader, err := body.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if len(progress) == 0 {
		t.Fatal("expected progress events")
	}
	last := progress[len(progress)-1]
	if last.Loaded != 5 || last.Total != 5 || !last.Known {
		t.Fatalf("last progress = %#v", last)
	}
}
