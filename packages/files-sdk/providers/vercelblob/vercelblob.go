package vercelblob

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/typeutil"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/urlutil"
)

type Access string

const (
	AccessPublic  Access = "public"
	AccessPrivate Access = "private"
)

const defaultDownloadTimeout = 5 * time.Minute

type Options struct {
	Token                  string
	OIDCToken              string
	StoreID                string
	Access                 Access
	AddRandomSuffix        bool
	AllowOverwrite         *bool
	DownloadTimeout        time.Duration
	DisableDownloadTimeout bool
	APIURL                 string
	HTTPClient             *http.Client
}

type Adapter struct {
	auth            blobAuth
	access          Access
	addRandomSuffix bool
	allowOverwrite  bool
	downloadTimeout time.Duration
	apiURL          string
	httpClient      *http.Client
	requestSeq      uint64
}

func New(opts Options) (*Adapter, error) {
	auth, err := resolveAuth(opts)
	if err != nil {
		return nil, err
	}
	access := opts.Access
	if access == "" {
		access = AccessPublic
	}
	if access != AccessPublic && access != AccessPrivate {
		return nil, files.NewError(files.ErrProvider, "vercel-blob adapter: access must be public or private", nil)
	}
	allowOverwrite := true
	if opts.AllowOverwrite != nil {
		allowOverwrite = *opts.AllowOverwrite
	}
	timeout := opts.DownloadTimeout
	if timeout <= 0 && !opts.DisableDownloadTimeout {
		timeout = defaultDownloadTimeout
	}
	apiURL := opts.APIURL
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Adapter{
		auth:            auth,
		access:          access,
		addRandomSuffix: opts.AddRandomSuffix,
		allowOverwrite:  allowOverwrite,
		downloadTimeout: timeout,
		apiURL:          trimRightSlash(apiURL),
		httpClient:      client,
	}, nil
}

func (a *Adapter) Name() string { return "vercel-blob" }

func (a *Adapter) Raw() any { return a.httpClient }

func (a *Adapter) Capabilities() files.AdapterCapabilities {
	return files.AdapterCapabilities{
		RangeRead:      a.access != AccessPrivate,
		UploadProgress: true,
		Delimiter:      true,
		CacheControl:   true,
		Resumable:      true,
		ServerSideCopy: true,
		SignedURL:      files.SignedURLCapability{Supported: false},
	}
}

func (a *Adapter) Upload(ctx context.Context, key string, body files.Body, opts files.UploadOptions) (files.UploadResult, error) {
	if err := validatePathname(key); err != nil {
		return files.UploadResult{}, err
	}
	contentType := typeutil.EffectiveContentType(opts.ContentType, body.ContentType())
	uploadBody := body
	if opts.OnProgress != nil {
		size, known := body.Size()
		opts.OnProgress(files.UploadProgress{Loaded: 0, Total: size, Known: known})
		uploadBody = files.BodyWithProgress(body, opts.OnProgress)
	}
	size, sizeKnown := uploadBody.Size()
	reader, err := uploadBody.Open(ctx)
	if err != nil {
		return files.UploadResult{}, err
	}
	defer reader.Close()

	headers := a.createBlobHeaders(contentType, opts.CacheControl)
	if opts.OnProgress != nil && sizeKnown {
		headers.Set("x-content-length", strconv.FormatInt(size, 10))
	}
	var result putBlobResponse
	contentLength := int64(-1)
	if sizeKnown {
		contentLength = size
	}
	if _, err := a.requestAPI(ctx, http.MethodPut, queryPath(url.Values{"pathname": []string{key}}), headers, reader, contentLength, &result); err != nil {
		return files.UploadResult{}, err
	}

	lastModified := time.Now()
	if !sizeKnown {
		headKey := first(result.Pathname, result.URL, key)
		head, err := a.headRaw(ctx, headKey)
		if err != nil {
			return files.UploadResult{}, err
		}
		size = head.Size
		if !head.UploadedAt.IsZero() {
			lastModified = head.UploadedAt
		}
	}
	if opts.OnProgress != nil {
		opts.OnProgress(files.UploadProgress{Loaded: size, Total: size, Known: true})
	}
	return files.UploadResult{
		Key:          first(result.Pathname, key),
		Size:         size,
		ContentType:  typeutil.EffectiveContentType(result.ContentType, contentType),
		ETag:         result.ETag,
		LastModified: lastModified,
	}, nil
}

func (a *Adapter) Download(ctx context.Context, key string, opts files.DownloadOptions) (files.StoredFile, error) {
	head, err := a.headRaw(ctx, key)
	if err != nil {
		return files.StoredFile{}, err
	}
	if a.access == AccessPrivate && opts.Range != nil {
		return files.StoredFile{}, files.NewError(files.ErrProvider, "vercel-blob: range downloads are not supported for private blobs", nil)
	}
	meta := head.meta(key)
	if a.access == AccessPrivate {
		body, header, err := a.fetchPrivateBody(ctx, key, nil)
		if err != nil {
			return files.StoredFile{}, err
		}
		meta.Size = firstInt64(contentLength(header), head.Size)
		meta.ContentType = typeutil.EffectiveContentType(header.Get("Content-Type"), head.ContentType)
		return files.NewStoredFile(meta, func(context.Context) (io.ReadCloser, error) { return body, nil }), nil
	}
	if head.URL == "" {
		return files.StoredFile{}, files.NewError(files.ErrProvider, "vercel-blob: missing public URL", nil)
	}
	body, header, err := a.fetchURL(ctx, head.URL, opts.Range, false)
	if err != nil {
		return files.StoredFile{}, err
	}
	if opts.Range != nil {
		meta.Size = firstInt64(contentLength(header), rangedSize(head.Size, *opts.Range))
	} else if got := contentLength(header); got > 0 {
		meta.Size = got
	}
	meta.ContentType = typeutil.EffectiveContentType(header.Get("Content-Type"), head.ContentType)
	return files.NewStoredFile(meta, func(context.Context) (io.ReadCloser, error) { return body, nil }), nil
}

func (a *Adapter) Head(ctx context.Context, key string, _ files.OperationOptions) (files.StoredFile, error) {
	head, err := a.headRaw(ctx, key)
	if err != nil {
		return files.StoredFile{}, err
	}
	meta := head.meta(key)
	return files.NewStoredFile(meta, func(ctx context.Context) (io.ReadCloser, error) {
		if a.access == AccessPrivate {
			body, _, err := a.fetchPrivateBody(ctx, key, nil)
			return body, err
		}
		if head.URL == "" {
			return nil, files.NewError(files.ErrProvider, "vercel-blob: missing public URL", nil)
		}
		body, _, err := a.fetchURL(ctx, head.URL, nil, false)
		return body, err
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

func (a *Adapter) Delete(ctx context.Context, key string, _ files.OperationOptions) error {
	return a.deleteKeys(ctx, []string{key})
}

func (a *Adapter) DeleteMany(ctx context.Context, keys []string, opts files.DeleteManyOptions) (files.DeleteManyResult, error) {
	if len(keys) == 0 {
		return files.DeleteManyResult{}, nil
	}
	if opts.StopOnError {
		out := files.DeleteManyResult{}
		for _, key := range keys {
			if err := a.Delete(ctx, key, files.OperationOptions{}); err != nil {
				out.Errors = append(out.Errors, files.DeleteManyError{Key: key, Error: files.WrapError(err, files.ErrProvider)})
				return out, nil
			}
			out.Deleted = append(out.Deleted, key)
		}
		return out, nil
	}
	if err := a.deleteKeys(ctx, keys); err != nil {
		mapped := files.WrapError(err, files.ErrProvider)
		out := files.DeleteManyResult{}
		for _, key := range keys {
			out.Errors = append(out.Errors, files.DeleteManyError{Key: key, Error: mapped})
		}
		return out, nil
	}
	return files.DeleteManyResult{Deleted: append([]string(nil), keys...)}, nil
}

func (a *Adapter) Copy(ctx context.Context, from string, to string, _ files.OperationOptions) error {
	if err := validatePathname(to); err != nil {
		return err
	}
	headers := a.createBlobHeaders("", "")
	_, err := a.requestAPI(ctx, http.MethodPut, queryPath(url.Values{
		"fromUrl":  []string{from},
		"pathname": []string{to},
	}), headers, nil, -1, nil)
	return err
}

func (a *Adapter) List(ctx context.Context, opts files.ListOptions) (files.ListResult, error) {
	if opts.Delimiter != "" && opts.Delimiter != "/" {
		return files.ListResult{}, files.NewError(files.ErrProvider, "vercel-blob: only the / delimiter is supported", nil)
	}
	params := url.Values{}
	if opts.Prefix != "" {
		params.Set("prefix", opts.Prefix)
	}
	if opts.Cursor != "" {
		params.Set("cursor", opts.Cursor)
	}
	if opts.Limit > 0 {
		params.Set("limit", strconv.FormatInt(int64(opts.Limit), 10))
	}
	if opts.Delimiter != "" {
		params.Set("mode", "folded")
	}
	var decoded listResponse
	if _, err := a.requestAPI(ctx, http.MethodGet, queryPath(params), nil, nil, -1, &decoded); err != nil {
		return files.ListResult{}, err
	}
	out := files.ListResult{Prefixes: append([]string(nil), decoded.Folders...)}
	if decoded.HasMore {
		out.Cursor = decoded.Cursor
	}
	for _, item := range decoded.Blobs {
		blob := item
		meta := blob.meta()
		out.Items = append(out.Items, files.NewStoredFile(meta, func(ctx context.Context) (io.ReadCloser, error) {
			if a.access == AccessPrivate {
				body, _, err := a.fetchPrivateBody(ctx, blob.Pathname, nil)
				return body, err
			}
			body, _, err := a.fetchURL(ctx, blob.URL, nil, false)
			return body, err
		}))
	}
	return out, nil
}

func (a *Adapter) URL(ctx context.Context, key string, opts files.URLOptions) (string, error) {
	if opts.ResponseContentDisposition != "" {
		return "", files.NewError(files.ErrProvider, "vercel-blob: responseContentDisposition is not supported", nil)
	}
	if a.access == AccessPrivate {
		return "", files.NewError(files.ErrProvider, "vercel-blob: url is not supported for private blobs. Use Download to read the body with credentials.", nil)
	}
	if a.auth.storeID != "" && !a.addRandomSuffix {
		return urlutil.JoinPublicURL("https://"+a.auth.storeID+".public.blob.vercel-storage.com", key), nil
	}
	head, err := a.headRaw(ctx, key)
	if err != nil {
		return "", err
	}
	if head.URL == "" {
		return "", files.NewError(files.ErrProvider, "vercel-blob: missing public URL", nil)
	}
	return head.URL, nil
}

func (a *Adapter) SignedUploadURL(context.Context, string, files.SignedUploadOptions) (files.SignedUpload, error) {
	return files.SignedUpload{}, files.NewError(files.ErrProvider, "vercel-blob: signed upload URLs are not available. Use Vercel client uploads with @vercel/blob/client.", nil)
}

func (a *Adapter) headRaw(ctx context.Context, keyOrURL string) (headBlob, error) {
	var decoded headResponse
	if _, err := a.requestAPI(ctx, http.MethodGet, queryPath(url.Values{"url": []string{keyOrURL}}), nil, nil, -1, &decoded); err != nil {
		return headBlob{}, err
	}
	return decoded.blob(), nil
}

func (a *Adapter) deleteKeys(ctx context.Context, keys []string) error {
	payload := deleteRequest{URLs: keys}
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	_, err := a.requestJSON(ctx, http.MethodPost, "/delete", headers, payload, nil)
	return err
}

func (a *Adapter) createBlobHeaders(contentType string, cacheControl string) http.Header {
	headers := http.Header{}
	headers.Set("x-vercel-blob-access", string(a.access))
	headers.Set("x-add-random-suffix", boolHeader(a.addRandomSuffix))
	headers.Set("x-allow-overwrite", boolHeader(a.allowOverwrite))
	if contentType != "" {
		headers.Set("x-content-type", contentType)
	}
	if maxAge, ok := parseCacheControlMaxAge(cacheControl); ok {
		headers.Set("x-cache-control-max-age", strconv.FormatInt(maxAge, 10))
	}
	return headers
}
