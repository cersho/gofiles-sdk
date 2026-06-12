package vercelblob

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	files "github.com/cersho/gofiles-sdk"
	"github.com/cersho/gofiles-sdk/internal/rangeutil"
	"github.com/cersho/gofiles-sdk/internal/typeutil"
	"github.com/cersho/gofiles-sdk/internal/urlutil"
)

const (
	defaultAPIURL = "https://vercel.com/api/blob"
	apiVersion    = "12"
	tokenPrefix   = "vercel_blob_rw_"
	storePrefix   = "store_"
)

var storeIDPattern = regexp.MustCompile(`^[A-Za-z0-9]{8,}$`)

type blobAuth struct {
	token   string
	storeID string
}

type putBlobResponse struct {
	URL                string `json:"url"`
	DownloadURL        string `json:"downloadUrl"`
	Pathname           string `json:"pathname"`
	ContentType        string `json:"contentType"`
	ContentDisposition string `json:"contentDisposition"`
	ETag               string `json:"etag"`
}

type headResponse struct {
	URL                string `json:"url"`
	DownloadURL        string `json:"downloadUrl"`
	Pathname           string `json:"pathname"`
	ContentType        string `json:"contentType"`
	ContentDisposition string `json:"contentDisposition"`
	CacheControl       string `json:"cacheControl"`
	ETag               string `json:"etag"`
	Size               int64  `json:"size"`
	UploadedAt         string `json:"uploadedAt"`
}

type headBlob struct {
	URL                string
	DownloadURL        string
	Pathname           string
	ContentType        string
	ContentDisposition string
	CacheControl       string
	ETag               string
	Size               int64
	UploadedAt         time.Time
}

type listResponse struct {
	Blobs   []listBlob `json:"blobs"`
	Folders []string   `json:"folders"`
	Cursor  string     `json:"cursor"`
	HasMore bool       `json:"hasMore"`
}

type listBlob struct {
	URL         string `json:"url"`
	DownloadURL string `json:"downloadUrl"`
	Pathname    string `json:"pathname"`
	Size        int64  `json:"size"`
	UploadedAt  string `json:"uploadedAt"`
	ETag        string `json:"etag"`
}

type deleteRequest struct {
	URLs []string `json:"urls"`
}

type apiErrorBody struct {
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func resolveAuth(opts Options) (blobAuth, error) {
	explicitToken := strings.TrimSpace(opts.Token)
	envToken := readEnv("BLOB_READ_WRITE_TOKEN")
	oidcToken := strings.TrimSpace(opts.OIDCToken)
	if oidcToken == "" {
		oidcToken = readEnv("VERCEL_OIDC_TOKEN")
	}
	rawStoreID := strings.TrimSpace(opts.StoreID)
	if rawStoreID == "" {
		rawStoreID = readEnv("BLOB_STORE_ID")
	}
	storeID, err := normalizeOptionalStoreID(rawStoreID)
	if err != nil {
		return blobAuth{}, err
	}
	if explicitToken != "" {
		if storeID == "" {
			storeID = deriveStoreIDFromToken(explicitToken)
		}
		return blobAuth{token: explicitToken, storeID: storeID}, nil
	}
	if oidcToken != "" && rawStoreID != "" {
		if storeID == "" {
			return blobAuth{}, files.NewError(files.ErrProvider, "vercel-blob adapter: storeId is not valid", nil)
		}
		return blobAuth{token: oidcToken, storeID: storeID}, nil
	}
	if strings.TrimSpace(opts.OIDCToken) != "" {
		return blobAuth{}, files.NewError(files.ErrProvider, "vercel-blob adapter: OIDCToken was passed but no StoreID was found. Pass StoreID or set BLOB_STORE_ID to use OIDC.", nil)
	}
	if envToken != "" {
		if storeID == "" {
			storeID = deriveStoreIDFromToken(envToken)
		}
		return blobAuth{token: envToken, storeID: storeID}, nil
	}
	return blobAuth{}, files.NewError(files.ErrProvider, "vercel-blob adapter: missing credentials. Pass Token, or OIDCToken plus StoreID, or set BLOB_READ_WRITE_TOKEN, or set both VERCEL_OIDC_TOKEN and BLOB_STORE_ID.", nil)
}

func (a *Adapter) requestJSON(ctx context.Context, method string, pathAndQuery string, headers http.Header, payload any, out any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, files.NewError(files.ErrProvider, err.Error(), err)
	}
	return a.requestAPI(ctx, method, pathAndQuery, headers, bytes.NewReader(data), int64(len(data)), out)
}

func (a *Adapter) requestAPI(ctx context.Context, method string, pathAndQuery string, headers http.Header, body io.Reader, contentLength int64, out any) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, a.apiURL+pathAndQuery, body)
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
		return nil, mapBlobError(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, files.NewError(files.ErrProvider, err.Error(), err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, blobStatusError(res.StatusCode, data)
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return nil, files.NewError(files.ErrProvider, err.Error(), err)
		}
	}
	return data, nil
}

func (a *Adapter) addAuthHeaders(req *http.Request) {
	req.Header.Set("x-api-version", apiVersion)
	req.Header.Set("x-api-blob-request-attempt", "0")
	req.Header.Set("x-vercel-blob-store-id", a.auth.storeID)
	req.Header.Set("x-api-blob-request-id", fmt.Sprintf("%s:%d:%d", a.auth.storeID, time.Now().UnixMilli(), atomic.AddUint64(&a.requestSeq, 1)))
	if a.auth.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.auth.token)
	}
}

func (a *Adapter) fetchURL(ctx context.Context, rawURL string, byteRange *files.ByteRange, authenticated bool) (io.ReadCloser, http.Header, error) {
	reqCtx, cancel := a.downloadContext(ctx)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		cancel()
		return nil, nil, files.NewError(files.ErrProvider, err.Error(), err)
	}
	if byteRange != nil {
		req.Header.Set("Range", rangeutil.Header(byteRange.Start, byteRange.End))
	}
	if authenticated && a.auth.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.auth.token)
	}
	res, err := a.httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, nil, mapBlobError(err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		res.Body.Close()
		cancel()
		return nil, nil, directStatusError(res.StatusCode, "vercel-blob download failed: "+res.Status)
	}
	if byteRange != nil && res.StatusCode != http.StatusPartialContent {
		res.Body.Close()
		cancel()
		return nil, nil, files.NewPermanentError(files.ErrProvider, fmt.Sprintf("vercel-blob: server ignored requested byte range (HTTP %d)", res.StatusCode), nil)
	}
	return cancelReadCloser{ReadCloser: res.Body, cancel: cancel}, res.Header, nil
}

func (a *Adapter) fetchPrivateBody(ctx context.Context, key string, byteRange *files.ByteRange) (io.ReadCloser, http.Header, error) {
	if a.auth.storeID == "" {
		return nil, nil, files.NewError(files.ErrProvider, "vercel-blob: private blob reads require a StoreID", nil)
	}
	rawURL := urlutil.JoinPublicURL("https://"+a.auth.storeID+".private.blob.vercel-storage.com", key)
	body, header, err := a.fetchURL(ctx, rawURL, byteRange, true)
	if err != nil {
		return nil, nil, err
	}
	return body, header, nil
}

func (a *Adapter) downloadContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if a.downloadTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, a.downloadTimeout)
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

func (h headResponse) blob() headBlob {
	return headBlob{
		URL:                h.URL,
		DownloadURL:        h.DownloadURL,
		Pathname:           h.Pathname,
		ContentType:        h.ContentType,
		ContentDisposition: h.ContentDisposition,
		CacheControl:       h.CacheControl,
		ETag:               h.ETag,
		Size:               h.Size,
		UploadedAt:         parseBlobTime(h.UploadedAt),
	}
}

func (h headBlob) meta(fallbackKey string) files.StoredFileMeta {
	return files.StoredFileMeta{
		Key:          first(h.Pathname, fallbackKey),
		Size:         h.Size,
		ContentType:  typeutil.EffectiveContentType(h.ContentType),
		LastModified: h.UploadedAt,
		ETag:         h.ETag,
	}
}

func (b listBlob) meta() files.StoredFileMeta {
	return files.StoredFileMeta{
		Key:          b.Pathname,
		Size:         b.Size,
		ContentType:  typeutil.GenericContentType,
		LastModified: parseBlobTime(b.UploadedAt),
		ETag:         b.ETag,
	}
}

func blobStatusError(status int, data []byte) *files.Error {
	message := strings.TrimSpace(string(data))
	codeFromBody := ""
	var decoded apiErrorBody
	if len(data) > 0 && json.Unmarshal(data, &decoded) == nil && decoded.Error != nil {
		codeFromBody = decoded.Error.Code
		if decoded.Error.Message != "" {
			message = decoded.Error.Message
		}
	}
	if message == "" {
		message = http.StatusText(status)
	}
	code := codeForStatus(status)
	if code == files.ErrProvider {
		code = codeForBlobCode(codeFromBody)
	}
	return files.NewError(code, message, nil)
}

func directStatusError(status int, message string) *files.Error {
	if status == http.StatusNotModified {
		return files.NewError(files.ErrNotFound, message, nil)
	}
	return files.NewError(codeForStatus(status), message, nil)
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

func codeForBlobCode(code string) files.ErrorCode {
	switch code {
	case "not_found", "store_not_found":
		return files.ErrNotFound
	case "forbidden", "not_allowed", "client_token_not_allowed", "client_token_expired", "oidc_environment_not_allowed":
		return files.ErrUnauthorized
	case "precondition_failed":
		return files.ErrConflict
	default:
		return files.ErrProvider
	}
}

func mapBlobError(err error) *files.Error {
	if err == nil {
		return nil
	}
	var fe *files.Error
	if errors.As(err, &fe) {
		return fe
	}
	return files.WrapError(err, files.ErrProvider)
}

func validatePathname(pathname string) error {
	if pathname == "" {
		return files.NewError(files.ErrProvider, "vercel-blob: pathname is required", nil)
	}
	if len(pathname) > 950 {
		return files.NewError(files.ErrProvider, "vercel-blob: pathname is too long, maximum length is 950", nil)
	}
	if strings.Contains(pathname, "//") {
		return files.NewError(files.ErrProvider, `vercel-blob: pathname cannot contain "//"`, nil)
	}
	return nil
}

func normalizeOptionalStoreID(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	candidate := value
	if strings.HasPrefix(candidate, storePrefix) {
		candidate = candidate[len(storePrefix):]
	}
	if !storeIDPattern.MatchString(candidate) {
		return "", files.NewError(files.ErrProvider, "vercel-blob adapter: storeId is not valid", nil)
	}
	return candidate, nil
}

func deriveStoreIDFromToken(token string) string {
	if !strings.HasPrefix(token, tokenPrefix) {
		return ""
	}
	afterPrefix := token[len(tokenPrefix):]
	sep := strings.Index(afterPrefix, "_")
	candidate := afterPrefix
	if sep >= 0 {
		candidate = afterPrefix[:sep]
	}
	if storeIDPattern.MatchString(candidate) {
		return candidate
	}
	return ""
}

func readEnv(name string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return ""
	}
	return value
}

func parseCacheControlMaxAge(header string) (int64, bool) {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.HasPrefix(strings.ToLower(part), "max-age=") {
			continue
		}
		value := strings.TrimSpace(part[len("max-age="):])
		seconds, err := strconv.ParseInt(value, 10, 64)
		if err != nil || seconds < 0 {
			return 0, false
		}
		return seconds, true
	}
	return 0, false
}

func parseBlobTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed
	}
	if parsed, err := http.ParseTime(value); err == nil {
		return parsed
	}
	return time.Time{}
}

func queryPath(values url.Values) string {
	encoded := values.Encode()
	if encoded == "" {
		return ""
	}
	return "?" + encoded
}

func boolHeader(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func contentLength(header http.Header) int64 {
	size, _ := strconv.ParseInt(header.Get("Content-Length"), 10, 64)
	return size
}

func rangedSize(fullSize int64, r files.ByteRange) int64 {
	return rangeutil.Size(fullSize, r.Start, r.End)
}

func trimRightSlash(value string) string {
	return strings.TrimRight(value, "/")
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
