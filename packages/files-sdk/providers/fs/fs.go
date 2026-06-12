package fs

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/typeutil"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/urlutil"
)

type Options struct {
	Root                string
	URLBaseURL          string
	DefaultURLExpiresIn time.Duration
}

type Adapter struct {
	root                string
	urlBaseURL          string
	defaultURLExpiresIn time.Duration
}

func New(opts Options) (*Adapter, error) {
	if opts.Root == "" {
		return nil, files.NewError(files.ErrProvider, "fs adapter: missing root", nil)
	}
	root, err := filepath.Abs(opts.Root)
	if err != nil {
		return nil, mapFSError(err)
	}
	expires := opts.DefaultURLExpiresIn
	if expires <= 0 {
		expires = files.DefaultURLExpiresIn
	}
	return &Adapter{root: root, urlBaseURL: opts.URLBaseURL, defaultURLExpiresIn: expires}, nil
}

func (a *Adapter) Name() string { return "fs" }

func (a *Adapter) Raw() any { return map[string]string{"root": a.root} }

func (a *Adapter) Root() string { return a.root }

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
	target, err := a.resolve(key)
	if err != nil {
		return files.UploadResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return files.UploadResult{}, mapFSError(err)
	}
	reader, err := body.Open(ctx)
	if err != nil {
		return files.UploadResult{}, err
	}
	defer reader.Close()
	tmp, err := os.CreateTemp(filepath.Dir(target), ".files-sdk-*")
	if err != nil {
		return files.UploadResult{}, mapFSError(err)
	}
	tmpPath := tmp.Name()
	hash := sha1.New()
	written, copyErr := copyContext(ctx, io.MultiWriter(tmp, hash), reader)
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return files.UploadResult{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return files.UploadResult{}, mapFSError(closeErr)
	}
	if err := os.Rename(tmpPath, target); err != nil {
		_ = os.Remove(tmpPath)
		return files.UploadResult{}, mapFSError(err)
	}
	contentType := typeutil.EffectiveContentType(opts.ContentType, body.ContentType(), mime.TypeByExtension(filepath.Ext(key)))
	meta := sidecar{
		ContentType:  contentType,
		CacheControl: opts.CacheControl,
		Metadata:     cloneMap(opts.Metadata),
		ETag:         hex.EncodeToString(hash.Sum(nil)),
		LastModified: time.Now(),
	}
	if err := writeSidecar(target, meta); err != nil {
		return files.UploadResult{}, err
	}
	return files.UploadResult{Key: key, Size: written, ContentType: contentType, ETag: meta.ETag, LastModified: meta.LastModified}, nil
}

func (a *Adapter) Download(_ context.Context, key string, opts files.DownloadOptions) (files.StoredFile, error) {
	target, err := a.resolve(key)
	if err != nil {
		return files.StoredFile{}, err
	}
	info, meta, err := a.stat(target)
	if err != nil {
		return files.StoredFile{}, err
	}
	size := info.Size()
	start := int64(0)
	length := size
	if opts.Range != nil {
		start = opts.Range.Start
		if start > size {
			length = 0
		} else {
			end := size - 1
			if opts.Range.End != nil && *opts.Range.End < end {
				end = *opts.Range.End
			}
			length = end - start + 1
		}
	}
	return files.NewStoredFile(files.StoredFileMeta{
		Key:          key,
		Size:         length,
		ContentType:  meta.ContentType,
		LastModified: meta.LastModified,
		ETag:         meta.ETag,
		Metadata:     cloneMap(meta.Metadata),
	}, func(context.Context) (io.ReadCloser, error) {
		f, err := os.Open(target)
		if err != nil {
			return nil, mapFSError(err)
		}
		if start > 0 {
			if _, err := f.Seek(start, io.SeekStart); err != nil {
				_ = f.Close()
				return nil, mapFSError(err)
			}
		}
		if opts.Range == nil {
			return f, nil
		}
		return limitedReadCloser{Reader: io.LimitReader(f, length), closer: f}, nil
	}), nil
}

func (a *Adapter) Head(_ context.Context, key string, _ files.OperationOptions) (files.StoredFile, error) {
	target, err := a.resolve(key)
	if err != nil {
		return files.StoredFile{}, err
	}
	info, meta, err := a.stat(target)
	if err != nil {
		return files.StoredFile{}, err
	}
	return files.NewStoredFile(files.StoredFileMeta{
		Key:          key,
		Size:         info.Size(),
		ContentType:  meta.ContentType,
		LastModified: meta.LastModified,
		ETag:         meta.ETag,
		Metadata:     cloneMap(meta.Metadata),
	}, func(context.Context) (io.ReadCloser, error) {
		f, err := os.Open(target)
		if err != nil {
			return nil, mapFSError(err)
		}
		return f, nil
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
	target, err := a.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return mapFSError(err)
	}
	_ = os.Remove(sidecarPath(target))
	return nil
}

func (a *Adapter) Copy(_ context.Context, from string, to string, _ files.OperationOptions) error {
	source, err := a.resolve(from)
	if err != nil {
		return err
	}
	dest, err := a.resolve(to)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return mapFSError(err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return mapFSError(err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return mapFSError(err)
	}
	meta, err := readSidecar(source)
	if err == nil {
		meta.LastModified = time.Now()
		if err := writeSidecar(dest, meta); err != nil {
			return err
		}
	}
	return nil
}

func (a *Adapter) Move(_ context.Context, from string, to string, _ files.OperationOptions) error {
	source, err := a.resolve(from)
	if err != nil {
		return err
	}
	dest, err := a.resolve(to)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return mapFSError(err)
	}
	if err := os.Rename(source, dest); err != nil {
		return mapFSError(err)
	}
	if _, err := os.Stat(sidecarPath(source)); err == nil {
		_ = os.Rename(sidecarPath(source), sidecarPath(dest))
	}
	return nil
}

func (a *Adapter) List(_ context.Context, opts files.ListOptions) (files.ListResult, error) {
	var keys []string
	if err := filepath.WalkDir(a.root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if isReserved(p) {
			return nil
		}
		rel, err := filepath.Rel(a.root, p)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if opts.Prefix == "" || strings.HasPrefix(key, opts.Prefix) {
			keys = append(keys, key)
		}
		return nil
	}); err != nil {
		return files.ListResult{}, mapFSError(err)
	}
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
		file, err := a.Head(context.Background(), key, files.OperationOptions{})
		if err != nil {
			return files.ListResult{}, err
		}
		result.Items = append(result.Items, file)
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

func (a *Adapter) URL(_ context.Context, key string, _ files.URLOptions) (string, error) {
	target, err := a.resolve(key)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(target); err != nil {
		return "", mapFSError(err)
	}
	if a.urlBaseURL != "" {
		return urlutil.JoinPublicURL(a.urlBaseURL, key), nil
	}
	return (&url.URL{Scheme: "file", Path: target}).String(), nil
}

func (a *Adapter) SignedUploadURL(_ context.Context, key string, opts files.SignedUploadOptions) (files.SignedUpload, error) {
	target, err := a.resolve(key)
	if err != nil {
		return files.SignedUpload{}, err
	}
	u := (&url.URL{Scheme: "file", Path: target}).String()
	q := url.Values{}
	expires := opts.ExpiresIn
	if expires <= 0 {
		expires = a.defaultURLExpiresIn
	}
	q.Set("expires", time.Now().Add(expires).Format(time.RFC3339))
	if opts.ContentType != "" {
		q.Set("content-type", opts.ContentType)
	}
	return files.SignedUpload{Method: "PUT", URL: u + "?" + q.Encode()}, nil
}

func (a *Adapter) resolve(key string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(key))
	target := filepath.Join(a.root, cleaned)
	resolved, err := filepath.Abs(target)
	if err != nil {
		return "", mapFSError(err)
	}
	rootWithSep := a.root
	if !strings.HasSuffix(rootWithSep, string(os.PathSeparator)) {
		rootWithSep += string(os.PathSeparator)
	}
	if resolved == a.root || !strings.HasPrefix(resolved, rootWithSep) {
		return "", files.NewError(files.ErrProvider, "fs: key escapes adapter root: "+key, nil)
	}
	if isReserved(resolved) {
		return "", files.NewError(files.ErrProvider, "fs: key uses a reserved adapter suffix: "+key, nil)
	}
	return resolved, nil
}

func (a *Adapter) stat(target string) (os.FileInfo, sidecar, error) {
	info, err := os.Stat(target)
	if err != nil {
		return nil, sidecar{}, mapFSError(err)
	}
	meta, err := readSidecar(target)
	if err != nil {
		meta = sidecar{ContentType: mime.TypeByExtension(filepath.Ext(target)), LastModified: info.ModTime()}
		if meta.ContentType == "" {
			meta.ContentType = typeutil.GenericContentType
		}
	}
	return info, meta, nil
}
