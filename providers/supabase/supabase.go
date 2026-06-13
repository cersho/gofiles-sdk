package supabase

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	files "github.com/cersho/gofiles-sdk"
	"github.com/cersho/gofiles-sdk/internal/envutil"
	"github.com/cersho/gofiles-sdk/internal/typeutil"
	"github.com/cersho/gofiles-sdk/internal/urlutil"
)

const defaultDownloadTimeout = 5 * time.Minute
const defaultListLimit int32 = 100

type Options struct {
	Bucket                 string
	URL                    string
	Key                    string
	BearerToken            string
	Public                 bool
	PublicBaseURL          string
	DefaultURLExpiresIn    time.Duration
	DownloadTimeout        time.Duration
	DisableDownloadTimeout bool
	HTTPClient             *http.Client
}

type Adapter struct {
	bucket              string
	storageURL          string
	key                 string
	bearerToken         string
	public              bool
	publicBaseURL       string
	defaultURLExpiresIn time.Duration
	downloadTimeout     time.Duration
	httpClient          *http.Client
}

func New(opts Options) (*Adapter, error) {
	if opts.Bucket == "" {
		return nil, files.NewError(files.ErrProvider, "supabase adapter: missing bucket. Pass Bucket.", nil)
	}
	projectURL := envutil.First(opts.URL, os.Getenv("SUPABASE_URL"), os.Getenv("NEXT_PUBLIC_SUPABASE_URL"))
	key := envutil.First(opts.Key, os.Getenv("SUPABASE_SERVICE_ROLE_KEY"), os.Getenv("SUPABASE_KEY"), os.Getenv("NEXT_PUBLIC_SUPABASE_ANON_KEY"))
	if projectURL == "" || key == "" {
		return nil, files.NewError(files.ErrProvider, "supabase adapter: missing credentials. Pass URL + Key or set SUPABASE_URL / NEXT_PUBLIC_SUPABASE_URL and SUPABASE_SERVICE_ROLE_KEY / SUPABASE_KEY / NEXT_PUBLIC_SUPABASE_ANON_KEY.", nil)
	}
	storageURL := normalizeStorageURL(projectURL)
	expires := opts.DefaultURLExpiresIn
	if expires <= 0 {
		expires = files.DefaultURLExpiresIn
	}
	timeout := opts.DownloadTimeout
	if timeout <= 0 && !opts.DisableDownloadTimeout {
		timeout = defaultDownloadTimeout
	}
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	bearer := opts.BearerToken
	return &Adapter{
		bucket:              opts.Bucket,
		storageURL:          storageURL,
		key:                 key,
		bearerToken:         bearer,
		public:              opts.Public,
		publicBaseURL:       opts.PublicBaseURL,
		defaultURLExpiresIn: expires,
		downloadTimeout:     timeout,
		httpClient:          client,
	}, nil
}

func (a *Adapter) Name() string { return "supabase" }

func (a *Adapter) Raw() any { return a.httpClient }

func (a *Adapter) Bucket() string { return a.bucket }

func (a *Adapter) Capabilities() files.AdapterCapabilities {
	return files.AdapterCapabilities{
		UploadProgress: true,
		Delimiter:      true,
		Metadata:       true,
		CacheControl:   true,
		Resumable:      true,
		ServerSideCopy: true,
		SignedURL:      files.SignedURLCapability{Supported: true},
	}
}

func (a *Adapter) Upload(ctx context.Context, key string, body files.Body, opts files.UploadOptions) (files.UploadResult, error) {
	if len(opts.Metadata) > 0 && opts.Control != nil {
		return files.UploadResult{}, files.NewError(files.ErrProvider, "supabase: resumable uploads do not support metadata", nil)
	}
	contentType := typeutil.EffectiveContentType(opts.ContentType, body.ContentType())
	uploadBody := body
	if opts.OnProgress != nil {
		size, known := body.Size()
		opts.OnProgress(files.UploadProgress{Loaded: 0, Total: size, Known: known})
		uploadBody = files.BodyWithProgress(body, opts.OnProgress)
	}
	reader, err := uploadBody.Open(ctx)
	if err != nil {
		return files.UploadResult{}, err
	}
	defer reader.Close()
	size, sizeKnown := uploadBody.Size()
	headers := http.Header{}
	headers.Set("Content-Type", contentType)
	headers.Set("x-upsert", "true")
	if opts.CacheControl != "" {
		headers.Set("Cache-Control", opts.CacheControl)
	}
	if len(opts.Metadata) > 0 {
		encoded, err := metadataHeader(opts.Metadata)
		if err != nil {
			return files.UploadResult{}, err
		}
		headers.Set("x-metadata", encoded)
	}
	contentLength := int64(-1)
	if sizeKnown {
		contentLength = size
	}
	if _, err := a.request(ctx, http.MethodPost, "/object/"+a.bucket+"/"+escapeKey(key), nil, headers, reader, contentLength, nil); err != nil {
		return files.UploadResult{}, err
	}
	if !sizeKnown {
		info, err := a.info(ctx, key)
		if err == nil {
			size = info.Size
		}
	}
	if opts.OnProgress != nil {
		opts.OnProgress(files.UploadProgress{Loaded: size, Total: size, Known: true})
	}
	return files.UploadResult{Key: key, Size: size, ContentType: contentType}, nil
}

func (a *Adapter) Download(ctx context.Context, key string, opts files.DownloadOptions) (files.StoredFile, error) {
	if opts.Range != nil {
		return files.StoredFile{}, files.NewError(files.ErrProvider, "supabase: range downloads are not supported by this adapter", nil)
	}
	body, header, err := a.fetchObject(ctx, key)
	if err != nil {
		return files.StoredFile{}, err
	}
	meta := metaFromHeaders(key, header)
	return files.NewStoredFile(meta, func(context.Context) (io.ReadCloser, error) { return body, nil }), nil
}

func (a *Adapter) Head(ctx context.Context, key string, _ files.OperationOptions) (files.StoredFile, error) {
	info, err := a.info(ctx, key)
	if err != nil {
		return files.StoredFile{}, err
	}
	meta := files.StoredFileMeta{
		Key:          key,
		Size:         info.Size,
		ContentType:  typeutil.EffectiveContentType(info.ContentType),
		LastModified: info.LastModified,
		ETag:         stripETag(info.ETag),
		Metadata:     stringifyMetadata(info.Metadata),
	}
	return files.NewStoredFile(meta, func(ctx context.Context) (io.ReadCloser, error) {
		body, _, err := a.fetchObject(ctx, key)
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
	payload := copyRequest{BucketID: a.bucket, SourceKey: from, DestinationKey: to}
	_, err := a.requestJSON(ctx, http.MethodPost, "/object/copy", nil, payload, nil)
	return err
}

func (a *Adapter) List(ctx context.Context, opts files.ListOptions) (files.ListResult, error) {
	if opts.Delimiter != "" && opts.Delimiter != "/" {
		return files.ListResult{}, files.NewError(files.ErrProvider, "supabase: only the / delimiter is supported", nil)
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	payload := listRequest{Limit: limit}
	if opts.Prefix != "" {
		payload.Prefix = opts.Prefix
	}
	if opts.Cursor != "" {
		payload.Cursor = opts.Cursor
	}
	if opts.Delimiter != "" {
		payload.WithDelimiter = true
	}
	var decoded listResponse
	if _, err := a.requestJSON(ctx, http.MethodPost, "/object/list-v2/"+a.bucket, nil, payload, &decoded); err != nil {
		return files.ListResult{}, err
	}
	out := files.ListResult{}
	if decoded.HasNext && decoded.NextCursor != "" {
		out.Cursor = decoded.NextCursor
	}
	for _, folder := range decoded.Folders {
		raw := first(folder.Key, joinPrefixName(opts.Prefix, folder.Name))
		if raw != "" && !strings.HasSuffix(raw, "/") {
			raw += "/"
		}
		out.Prefixes = append(out.Prefixes, raw)
	}
	for _, object := range decoded.Objects {
		key := first(object.Key, object.Name)
		if opts.Delimiter != "" && object.Key == "" {
			key = joinPrefixName(opts.Prefix, object.Name)
		}
		obj := object
		out.Items = append(out.Items, files.NewStoredFile(files.StoredFileMeta{
			Key:          key,
			Size:         firstInt64(obj.Metadata.Size, obj.Metadata.ContentLength),
			ContentType:  typeutil.EffectiveContentType(obj.Metadata.MimeType),
			LastModified: parseAnyTime(obj.Metadata.LastModified),
			ETag:         stripETag(obj.Metadata.ETag),
			Metadata:     stringifyMetadata(obj.Metadata.Extra),
		}, func(ctx context.Context) (io.ReadCloser, error) {
			body, _, err := a.fetchObject(ctx, key)
			return body, err
		}))
	}
	return out, nil
}

func (a *Adapter) URL(ctx context.Context, key string, opts files.URLOptions) (string, error) {
	wantsDisposition := opts.ResponseContentDisposition != ""
	if a.publicBaseURL != "" && !wantsDisposition {
		return urlutil.JoinPublicURL(a.publicBaseURL, key), nil
	}
	if a.public && !wantsDisposition {
		return a.storageURL + "/object/public/" + a.bucket + "/" + escapeKey(key), nil
	}
	expires := opts.ExpiresIn
	if expires <= 0 {
		expires = a.defaultURLExpiresIn
	}
	payload := signedURLRequest{ExpiresIn: int64(expires / time.Second)}
	if wantsDisposition {
		download, err := downloadOptionFor(opts.ResponseContentDisposition)
		if err != nil {
			return "", err
		}
		payload.Download = download
	}
	var decoded signedURLResponse
	if _, err := a.requestJSON(ctx, http.MethodPost, "/object/sign/"+a.bucket+"/"+escapeKey(key), nil, payload, &decoded); err != nil {
		return "", err
	}
	got := first(decoded.SignedURL, decoded.SignedURLAlt)
	if got == "" {
		return "", files.NewError(files.ErrProvider, "supabase: signed URL response missing signedURL", nil)
	}
	return a.absoluteURL(got), nil
}

func (a *Adapter) SignedUploadURL(ctx context.Context, key string, opts files.SignedUploadOptions) (files.SignedUpload, error) {
	if opts.MaxSize != nil || opts.MinSize != nil {
		return files.SignedUpload{}, files.NewError(files.ErrProvider, "supabase: maxSize/minSize are not supported by Supabase signed upload URLs", nil)
	}
	var decoded signedUploadResponse
	if _, err := a.requestJSON(ctx, http.MethodPost, "/object/upload/sign/"+a.bucket+"/"+escapeKey(key), nil, map[string]bool{"upsert": true}, &decoded); err != nil {
		return files.SignedUpload{}, err
	}
	signed := first(decoded.SignedURL, decoded.SignedURLAlt, decoded.URL)
	if signed == "" && decoded.Token != "" {
		signed = "/object/upload/sign/" + a.bucket + "/" + escapeKey(key) + "?token=" + url.QueryEscape(decoded.Token)
	}
	if signed == "" {
		return files.SignedUpload{}, files.NewError(files.ErrProvider, "supabase: signed upload response missing URL", nil)
	}
	headers := map[string]string{"x-upsert": "true"}
	if opts.ContentType != "" {
		headers["Content-Type"] = opts.ContentType
	}
	return files.SignedUpload{Method: http.MethodPut, URL: a.absoluteURL(signed), Headers: headers}, nil
}

func (a *Adapter) info(ctx context.Context, key string) (objectInfo, error) {
	var decoded objectInfo
	if _, err := a.requestJSON(ctx, http.MethodGet, "/object/info/"+a.bucket+"/"+escapeKey(key), nil, nil, &decoded); err != nil {
		return objectInfo{}, err
	}
	return decoded, nil
}

func (a *Adapter) deleteKeys(ctx context.Context, keys []string) error {
	_, err := a.requestJSON(ctx, http.MethodDelete, "/object/"+a.bucket, nil, deleteRequest{Prefixes: keys}, nil)
	return err
}

func (a *Adapter) fetchObject(ctx context.Context, key string) (io.ReadCloser, http.Header, error) {
	reqCtx, cancel := a.downloadContext(ctx)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, a.storageURL+"/object/"+a.bucket+"/"+escapeKey(key), nil)
	if err != nil {
		cancel()
		return nil, nil, files.NewError(files.ErrProvider, err.Error(), err)
	}
	a.addAuthHeaders(req)
	res, err := a.httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, nil, mapSupabaseError(err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		cancel()
		return nil, nil, statusError(res.StatusCode, data)
	}
	return cancelReadCloser{ReadCloser: res.Body, cancel: cancel}, res.Header, nil
}

func (a *Adapter) requestJSON(ctx context.Context, method string, path string, headers http.Header, payload any, out any) ([]byte, error) {
	var body io.Reader
	contentLength := int64(-1)
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, files.NewError(files.ErrProvider, err.Error(), err)
		}
		body = bytes.NewReader(data)
		contentLength = int64(len(data))
		if headers == nil {
			headers = http.Header{}
		}
		headers.Set("Content-Type", "application/json")
	}
	return a.request(ctx, method, path, nil, headers, body, contentLength, out)
}

func (a *Adapter) request(ctx context.Context, method string, path string, query url.Values, headers http.Header, body io.Reader, contentLength int64, out any) ([]byte, error) {
	target := a.storageURL + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, files.NewError(files.ErrProvider, err.Error(), err)
	}
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}
	for key, values := range headers {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	a.addAuthHeaders(req)
	res, err := a.httpClient.Do(req)
	if err != nil {
		return nil, mapSupabaseError(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, files.NewError(files.ErrProvider, err.Error(), err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, statusError(res.StatusCode, data)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return nil, files.NewError(files.ErrProvider, err.Error(), err)
		}
	}
	return data, nil
}

func (a *Adapter) addAuthHeaders(req *http.Request) {
	req.Header.Set("apikey", a.key)
	if token := a.authBearer(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func (a *Adapter) authBearer() string {
	if a.bearerToken != "" {
		return a.bearerToken
	}
	if isJWTLike(a.key) {
		return a.key
	}
	return ""
}

func (a *Adapter) downloadContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if a.downloadTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, a.downloadTimeout)
}

func (a *Adapter) absoluteURL(value string) string {
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return value
	}
	if strings.HasPrefix(value, "/") {
		return a.storageURL + value
	}
	return a.storageURL + "/" + value
}

type objectInfo struct {
	Size         int64          `json:"size"`
	ContentType  string         `json:"contentType"`
	ETag         string         `json:"etag"`
	LastModified time.Time      `json:"-"`
	Metadata     map[string]any `json:"metadata"`
	RawModified  any            `json:"lastModified"`
}

func (i *objectInfo) UnmarshalJSON(data []byte) error {
	type alias objectInfo
	aux := struct {
		*alias
		LastModified any `json:"lastModified"`
	}{alias: (*alias)(i)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	i.RawModified = aux.LastModified
	i.LastModified = parseAnyTime(aux.LastModified)
	return nil
}

type listRequest struct {
	Limit         int32  `json:"limit"`
	Prefix        string `json:"prefix,omitempty"`
	Cursor        string `json:"cursor,omitempty"`
	WithDelimiter bool   `json:"with_delimiter,omitempty"`
}

type listResponse struct {
	Objects    []listObject `json:"objects"`
	Folders    []listFolder `json:"folders"`
	HasNext    bool         `json:"hasNext"`
	NextCursor string       `json:"nextCursor"`
}

type listObject struct {
	Key      string       `json:"key"`
	Name     string       `json:"name"`
	Metadata listMetadata `json:"metadata"`
}

type listFolder struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type listMetadata struct {
	ETag          string         `json:"eTag"`
	Size          int64          `json:"size"`
	MimeType      string         `json:"mimetype"`
	CacheControl  string         `json:"cacheControl"`
	LastModified  any            `json:"lastModified"`
	ContentLength int64          `json:"contentLength"`
	Extra         map[string]any `json:"-"`
}

func (m *listMetadata) UnmarshalJSON(data []byte) error {
	type alias listMetadata
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	aux := alias{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	*m = listMetadata(aux)
	delete(raw, "eTag")
	delete(raw, "size")
	delete(raw, "mimetype")
	delete(raw, "cacheControl")
	delete(raw, "lastModified")
	delete(raw, "contentLength")
	m.Extra = raw
	return nil
}

type deleteRequest struct {
	Prefixes []string `json:"prefixes"`
}

type copyRequest struct {
	BucketID       string `json:"bucketId"`
	SourceKey      string `json:"sourceKey"`
	DestinationKey string `json:"destinationKey"`
}

type signedURLRequest struct {
	ExpiresIn int64 `json:"expiresIn"`
	Download  any   `json:"download,omitempty"`
}

type signedURLResponse struct {
	SignedURL    string `json:"signedURL"`
	SignedURLAlt string `json:"signedUrl"`
}

type signedUploadResponse struct {
	SignedURL    string `json:"signedURL"`
	SignedURLAlt string `json:"signedUrl"`
	URL          string `json:"url"`
	Token        string `json:"token"`
}

type apiErrorBody struct {
	Error       string `json:"error"`
	Message     string `json:"message"`
	StatusCode  any    `json:"statusCode"`
	Code        string `json:"code"`
	Description string `json:"description"`
}

func statusError(status int, data []byte) *files.Error {
	message := strings.TrimSpace(string(data))
	providerCode := ""
	var decoded apiErrorBody
	if len(data) > 0 && json.Unmarshal(data, &decoded) == nil {
		message = first(decoded.Message, decoded.Error, decoded.Description, message)
		providerCode = first(decoded.Code, stringStatusCode(decoded.StatusCode))
	}
	if message == "" {
		message = http.StatusText(status)
	}
	code := codeForStatus(status)
	if code == files.ErrProvider {
		code = codeForSupabaseCode(providerCode)
	}
	return files.NewError(code, message, nil)
}

func mapSupabaseError(err error) *files.Error {
	if err == nil {
		return nil
	}
	var fe *files.Error
	if errors.As(err, &fe) {
		return fe
	}
	return files.WrapError(err, files.ErrProvider)
}

func codeForStatus(status int) files.ErrorCode {
	switch status {
	case http.StatusNotFound:
		return files.ErrNotFound
	case http.StatusUnauthorized, http.StatusForbidden:
		return files.ErrUnauthorized
	case http.StatusConflict, http.StatusPreconditionFailed:
		return files.ErrConflict
	default:
		return files.ErrProvider
	}
}

func codeForSupabaseCode(code string) files.ErrorCode {
	switch code {
	case "NotFound", "NoSuchKey":
		return files.ErrNotFound
	case "InvalidJWT", "Unauthorized", "AccessDenied", "InvalidKey":
		return files.ErrUnauthorized
	case "Duplicate", "AlreadyExists":
		return files.ErrConflict
	default:
		return files.ErrProvider
	}
}

func normalizeStorageURL(raw string) string {
	trimmed := strings.TrimRight(raw, "/")
	if strings.HasSuffix(trimmed, "/storage/v1") {
		return trimmed
	}
	return trimmed + "/storage/v1"
}

func isJWTLike(value string) bool {
	return strings.Count(value, ".") == 2
}

func metadataHeader(metadata map[string]string) (string, error) {
	data, err := json.Marshal(metadata)
	if err != nil {
		return "", files.NewError(files.ErrProvider, err.Error(), err)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func downloadOptionFor(disposition string) (any, error) {
	typePart, params, _ := strings.Cut(disposition, ";")
	if strings.TrimSpace(strings.ToLower(typePart)) != "attachment" {
		return nil, files.NewError(files.ErrProvider, fmt.Sprintf("supabase: responseContentDisposition %q is not supported", disposition), nil)
	}
	for _, param := range strings.Split(params, ";") {
		key, value, ok := strings.Cut(param, "=")
		if !ok || strings.TrimSpace(strings.ToLower(key)) != "filename" {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) && len(value) >= 2 {
			value = value[1 : len(value)-1]
		}
		return value, nil
	}
	return true, nil
}

func metaFromHeaders(key string, header http.Header) files.StoredFileMeta {
	size, _ := strconv.ParseInt(header.Get("Content-Length"), 10, 64)
	lastModified := time.Time{}
	if raw := header.Get("Last-Modified"); raw != "" {
		if parsed, err := http.ParseTime(raw); err == nil {
			lastModified = parsed
		}
	}
	return files.StoredFileMeta{
		Key:          key,
		Size:         size,
		ContentType:  typeutil.EffectiveContentType(header.Get("Content-Type")),
		LastModified: lastModified,
		ETag:         stripETag(header.Get("ETag")),
	}
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (c cancelReadCloser) Close() error {
	err := c.ReadCloser.Close()
	c.cancel()
	return err
}

func escapeKey(key string) string {
	return urlutil.EscapeSegments(key)
}

func stripETag(value string) string {
	return strings.Trim(value, `"`)
}

func stringifyMetadata(metadata map[string]any) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range metadata {
		if value == nil {
			continue
		}
		if str, ok := value.(string); ok {
			out[key] = str
			continue
		}
		data, err := json.Marshal(value)
		if err == nil {
			out[key] = string(data)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseAnyTime(value any) time.Time {
	switch v := value.(type) {
	case string:
		if parsed, err := time.Parse(time.RFC3339Nano, v); err == nil {
			return parsed
		}
		if parsed, err := http.ParseTime(v); err == nil {
			return parsed
		}
	case float64:
		return time.UnixMilli(int64(v))
	case int64:
		return time.UnixMilli(v)
	case json.Number:
		n, _ := v.Int64()
		return time.UnixMilli(n)
	}
	return time.Time{}
}

func stringStatusCode(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		return strconv.Itoa(int(v))
	default:
		return ""
	}
}

func joinPrefixName(prefix string, name string) string {
	if prefix == "" {
		return name
	}
	return strings.TrimRight(prefix, "/") + "/" + name
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
		if value != 0 {
			return value
		}
	}
	return 0
}
