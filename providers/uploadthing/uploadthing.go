package uploadthing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	files "github.com/cersho/gofiles-sdk"
	"github.com/cersho/gofiles-sdk/internal/rangeutil"
	"github.com/cersho/gofiles-sdk/internal/typeutil"
)

type ACL string

const (
	ACLPublicRead ACL = "public-read"
	ACLPrivate    ACL = "private"
)

type Options struct {
	Token               string
	ACL                 ACL
	Slug                string
	DefaultURLExpiresIn time.Duration
	DownloadTimeout     time.Duration
	Region              string
	APIURL              string
	HTTPClient          *http.Client
}

type Adapter struct {
	token               string
	apiKey              string
	appID               string
	region              string
	acl                 ACL
	slug                string
	defaultURLExpiresIn time.Duration
	downloadTimeout     time.Duration
	apiURL              string
	httpClient          *http.Client
}

type decodedToken struct {
	APIKey  string   `json:"apiKey"`
	AppID   string   `json:"appId"`
	Regions []string `json:"regions"`
}

func New(opts Options) (*Adapter, error) {
	token := first(opts.Token, os.Getenv("UPLOADTHING_TOKEN"))
	if token == "" {
		return nil, files.NewError(files.ErrProvider, "uploadthing adapter: missing token. Pass Token or set UPLOADTHING_TOKEN.", nil)
	}
	decoded, err := decodeToken(token)
	if err != nil {
		return nil, err
	}
	acl := opts.ACL
	if acl == "" {
		acl = ACLPublicRead
	}
	region := opts.Region
	if region == "" && len(decoded.Regions) > 0 {
		region = decoded.Regions[0]
	}
	if region == "" {
		region = "sea1"
	}
	expires := opts.DefaultURLExpiresIn
	if expires <= 0 {
		expires = files.DefaultURLExpiresIn
	}
	timeout := opts.DownloadTimeout
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	apiURL := opts.APIURL
	if apiURL == "" {
		apiURL = "https://api.uploadthing.com"
	}
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Adapter{
		token:               token,
		apiKey:              decoded.APIKey,
		appID:               decoded.AppID,
		region:              region,
		acl:                 acl,
		slug:                opts.Slug,
		defaultURLExpiresIn: expires,
		downloadTimeout:     timeout,
		apiURL:              strings.TrimRight(apiURL, "/"),
		httpClient:          client,
	}, nil
}

func (a *Adapter) Name() string { return "uploadthing" }

func (a *Adapter) Raw() any { return a.httpClient }

func (a *Adapter) Capabilities() files.AdapterCapabilities {
	return files.AdapterCapabilities{
		RangeRead:      true,
		ServerSideCopy: false,
		SignedURL:      files.SignedURLCapability{Supported: true, MaxExpiresIn: 7 * 24 * time.Hour},
	}
}

func (a *Adapter) Upload(ctx context.Context, key string, body files.Body, opts files.UploadOptions) (files.UploadResult, error) {
	contentType := typeutil.EffectiveContentType(opts.ContentType, body.ContentType())
	size, _ := body.Size()
	signed, err := a.signedUploadURL(ctx, key, contentType, size, a.defaultURLExpiresIn)
	if err != nil {
		return files.UploadResult{}, err
	}
	reader, err := body.Open(ctx)
	if err != nil {
		return files.UploadResult{}, err
	}
	defer reader.Close()
	resp, err := a.putMultipart(ctx, signed.URL, basename(key), contentType, reader)
	if err != nil {
		return files.UploadResult{}, err
	}
	if resp.FileHash == "" {
		resp.FileHash = resp.Hash
	}
	return files.UploadResult{
		Key:          key,
		Size:         firstInt64(size, resp.Size),
		ContentType:  first(contentType, resp.Type),
		ETag:         resp.FileHash,
		LastModified: time.Now(),
	}, nil
}

func (a *Adapter) Download(ctx context.Context, key string, opts files.DownloadOptions) (files.StoredFile, error) {
	resolved, err := a.resolveFetchURL(ctx, key)
	if err != nil {
		return files.StoredFile{}, err
	}
	reqCtx, cancel := a.timeoutContext(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, resolved, nil)
	if err != nil {
		return files.StoredFile{}, files.NewError(files.ErrProvider, err.Error(), err)
	}
	if opts.Range != nil {
		req.Header.Set("Range", rangeHeader(*opts.Range))
	}
	res, err := a.httpClient.Do(req)
	if err != nil {
		return files.StoredFile{}, mapUploadThingError(err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		res.Body.Close()
		return files.StoredFile{}, statusError(res.StatusCode, "uploadthing download failed: "+res.Status)
	}
	if opts.Range != nil && res.StatusCode != http.StatusPartialContent {
		res.Body.Close()
		return files.StoredFile{}, files.NewPermanentError(files.ErrProvider, fmt.Sprintf("uploadthing: server ignored requested byte range (HTTP %d)", res.StatusCode), nil)
	}
	meta := metaFromHeaders(key, res.Header)
	return files.NewStoredFile(meta, func(context.Context) (io.ReadCloser, error) { return res.Body, nil }), nil
}

func (a *Adapter) Head(ctx context.Context, key string, _ files.OperationOptions) (files.StoredFile, error) {
	resolved, err := a.resolveFetchURL(ctx, key)
	if err != nil {
		return files.StoredFile{}, err
	}
	reqCtx, cancel := a.timeoutContext(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodHead, resolved, nil)
	if err != nil {
		return files.StoredFile{}, files.NewError(files.ErrProvider, err.Error(), err)
	}
	res, err := a.httpClient.Do(req)
	if err != nil {
		return files.StoredFile{}, mapUploadThingError(err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return files.StoredFile{}, statusError(res.StatusCode, "uploadthing head failed: "+res.Status)
	}
	meta := metaFromHeaders(key, res.Header)
	return files.NewStoredFile(meta, func(ctx context.Context) (io.ReadCloser, error) {
		return a.fetchBody(ctx, resolved)
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
	_, err := a.requestAPI(ctx, "/v6/deleteFiles", map[string]any{"customIds": []string{key}}, nil)
	return err
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
	_, err := a.requestAPI(ctx, "/v6/deleteFiles", map[string]any{"customIds": keys}, nil)
	if err != nil {
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
	src, err := a.Download(ctx, from, files.DownloadOptions{})
	if err != nil {
		return err
	}
	reader, err := src.Open(ctx)
	if err != nil {
		return err
	}
	defer reader.Close()
	_, err = a.Upload(ctx, to, files.ReaderBody(reader), files.UploadOptions{ContentType: src.ContentType})
	return err
}

func (a *Adapter) List(ctx context.Context, opts files.ListOptions) (files.ListResult, error) {
	offset := 0
	if opts.Cursor != "" {
		parsed, err := strconv.Atoi(opts.Cursor)
		if err != nil {
			return files.ListResult{}, files.NewError(files.ErrProvider, "uploadthing cursor must be a numeric offset", err)
		}
		offset = parsed
	}
	body := map[string]any{"offset": offset}
	if opts.Limit > 0 {
		body["limit"] = opts.Limit
	}
	var decoded listFilesResponse
	if _, err := a.requestAPI(ctx, "/v6/listFiles", body, &decoded); err != nil {
		return files.ListResult{}, err
	}
	result := files.ListResult{}
	for _, item := range decoded.Files {
		key := item.CustomID
		if key == "" {
			key = item.Key
		}
		if opts.Prefix != "" && !strings.HasPrefix(key, opts.Prefix) {
			continue
		}
		result.Items = append(result.Items, files.NewStoredFile(files.StoredFileMeta{
			Key:          key,
			Size:         item.Size,
			ContentType:  "application/octet-stream",
			LastModified: time.UnixMilli(item.UploadedAt),
		}, func(ctx context.Context) (io.ReadCloser, error) {
			resolved, err := a.resolveFetchURL(ctx, key)
			if err != nil {
				return nil, err
			}
			return a.fetchBody(ctx, resolved)
		}))
	}
	if decoded.HasMore {
		result.Cursor = strconv.Itoa(offset + len(decoded.Files))
	}
	return result, nil
}

func (a *Adapter) URL(ctx context.Context, key string, opts files.URLOptions) (string, error) {
	if opts.ResponseContentDisposition != "" {
		return "", files.NewError(files.ErrProvider, "uploadthing: responseContentDisposition is not supported", nil)
	}
	if a.acl == ACLPublicRead {
		return a.publicURL(key), nil
	}
	expires := opts.ExpiresIn
	if expires <= 0 {
		expires = a.defaultURLExpiresIn
	}
	return a.privateSignedURL(key, expires)
}

func (a *Adapter) SignedUploadURL(ctx context.Context, key string, opts files.SignedUploadOptions) (files.SignedUpload, error) {
	expires := opts.ExpiresIn
	if expires <= 0 {
		expires = a.defaultURLExpiresIn
	}
	return a.signedUploadURL(ctx, key, opts.ContentType, derefInt64(opts.MaxSize), expires)
}

func (a *Adapter) signedUploadURL(_ context.Context, key string, contentType string, size int64, expires time.Duration) (files.SignedUpload, error) {
	fileKey, err := randomFileKey()
	if err != nil {
		return files.SignedUpload{}, files.NewError(files.ErrProvider, "uploadthing: failed to generate file key", err)
	}
	u, err := url.Parse("https://" + a.region + ".ingest.uploadthing.com/" + fileKey)
	if err != nil {
		return files.SignedUpload{}, files.NewError(files.ErrProvider, err.Error(), err)
	}
	q := u.Query()
	q.Set("expires", strconv.FormatInt(time.Now().Add(expires).UnixMilli(), 10))
	q.Set("x-ut-identifier", a.appID)
	q.Set("x-ut-file-name", basename(key))
	if size > 0 {
		q.Set("x-ut-file-size", strconv.FormatInt(size, 10))
	}
	if a.slug != "" {
		q.Set("x-ut-slug", a.slug)
	}
	if contentType != "" {
		q.Set("x-ut-file-type", contentType)
	}
	q.Set("x-ut-custom-id", key)
	q.Set("x-ut-acl", string(a.acl))
	q.Set("x-ut-content-disposition", "inline")
	u.RawQuery = q.Encode()
	signature := hmacSHA256Hex(u.String(), a.apiKey)
	q.Set("signature", "hmac-sha256="+signature)
	u.RawQuery = q.Encode()
	return files.SignedUpload{Method: http.MethodPut, URL: u.String()}, nil
}

func (a *Adapter) privateSignedURL(key string, expires time.Duration) (string, error) {
	if expires <= 0 {
		expires = a.defaultURLExpiresIn
	}
	if expires > 7*24*time.Hour {
		return "", files.NewError(files.ErrProvider, "uploadthing: expiresIn must be less than 7 days", nil)
	}
	u, err := url.Parse("https://" + a.appID + ".ufs.sh/f/" + url.PathEscape(key))
	if err != nil {
		return "", files.NewError(files.ErrProvider, err.Error(), err)
	}
	q := u.Query()
	q.Set("expires", strconv.FormatInt(time.Now().Add(expires).UnixMilli(), 10))
	u.RawQuery = q.Encode()
	signature := hmacSHA256Hex(u.String(), a.apiKey)
	q.Set("signature", "hmac-sha256="+signature)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func (a *Adapter) publicURL(key string) string {
	return "https://" + a.appID + ".ufs.sh/f/" + url.PathEscape(key)
}

func (a *Adapter) resolveFetchURL(ctx context.Context, key string) (string, error) {
	if a.acl == ACLPublicRead {
		return a.publicURL(key), nil
	}
	return a.privateSignedURL(key, a.defaultURLExpiresIn)
}

func (a *Adapter) requestAPI(ctx context.Context, endpoint string, body any, out any) ([]byte, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, files.NewError(files.ErrProvider, err.Error(), err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.apiURL+endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, files.NewError(files.ErrProvider, err.Error(), err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-uploadthing-version", "7.0.0")
	req.Header.Set("x-uploadthing-be-adapter", "server-sdk")
	req.Header.Set("x-uploadthing-api-key", a.apiKey)
	res, err := a.httpClient.Do(req)
	if err != nil {
		return nil, mapUploadThingError(err)
	}
	defer res.Body.Close()
	payload, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, files.NewError(files.ErrProvider, err.Error(), err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, statusError(res.StatusCode, string(payload))
	}
	if out != nil {
		if err := json.Unmarshal(payload, out); err != nil {
			return nil, files.NewError(files.ErrProvider, err.Error(), err)
		}
	}
	return payload, nil
}

func (a *Adapter) putMultipart(ctx context.Context, uploadURL string, name string, contentType string, reader io.Reader) (uploadResponse, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		defer pw.Close()
		defer writer.Close()
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="file"; filename="`+escapeQuotes(name)+`"`)
		header.Set("Content-Type", contentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, reader); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
	}()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, pr)
	if err != nil {
		return uploadResponse{}, files.NewError(files.ErrProvider, err.Error(), err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Range", "bytes=0-")
	req.Header.Set("x-uploadthing-version", "7.0.0")
	res, err := a.httpClient.Do(req)
	if err != nil {
		return uploadResponse{}, mapUploadThingError(err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return uploadResponse{}, files.NewError(files.ErrProvider, err.Error(), err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return uploadResponse{}, statusError(res.StatusCode, string(data))
	}
	var out uploadResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return uploadResponse{}, files.NewError(files.ErrProvider, err.Error(), err)
	}
	return out, nil
}

func (a *Adapter) fetchBody(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	reqCtx, cancel := a.timeoutContext(ctx)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		cancel()
		return nil, files.NewError(files.ErrProvider, err.Error(), err)
	}
	res, err := a.httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, mapUploadThingError(err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		cancel()
		res.Body.Close()
		return nil, statusError(res.StatusCode, "uploadthing fetch failed: "+res.Status)
	}
	return cancelReadCloser{ReadCloser: res.Body, cancel: cancel}, nil
}

func (a *Adapter) timeoutContext(ctx context.Context) (context.Context, context.CancelFunc) {
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

type uploadResponse struct {
	URL          string `json:"url"`
	AppURL       string `json:"appUrl"`
	UFSURL       string `json:"ufsUrl"`
	FileHash     string `json:"fileHash"`
	Hash         string `json:"hash"`
	Size         int64  `json:"size"`
	Type         string `json:"type"`
	LastModified int64  `json:"lastModified"`
}

type listFilesResponse struct {
	HasMore bool `json:"hasMore"`
	Files   []struct {
		ID         string `json:"id"`
		CustomID   string `json:"customId"`
		Key        string `json:"key"`
		Name       string `json:"name"`
		Size       int64  `json:"size"`
		Status     string `json:"status"`
		UploadedAt int64  `json:"uploadedAt"`
	} `json:"files"`
}

func decodeToken(token string) (decodedToken, error) {
	data, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return decodedToken{}, files.NewError(files.ErrProvider, "uploadthing: UPLOADTHING_TOKEN is not valid base64", err)
	}
	var decoded decodedToken
	if err := json.Unmarshal(data, &decoded); err != nil {
		return decodedToken{}, files.NewError(files.ErrProvider, "uploadthing: UPLOADTHING_TOKEN does not decode to JSON", err)
	}
	if decoded.APIKey == "" || decoded.AppID == "" {
		return decodedToken{}, files.NewError(files.ErrProvider, "uploadthing: UPLOADTHING_TOKEN missing apiKey or appId", nil)
	}
	return decoded, nil
}

func hmacSHA256Hex(message string, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func randomFileKey() (string, error) {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, len(buf))
	for i, b := range buf {
		out[i] = chars[int(b)%len(chars)]
	}
	return string(out), nil
}

func rangeHeader(r files.ByteRange) string {
	return rangeutil.Header(r.Start, r.End)
}

func metaFromHeaders(key string, header http.Header) files.StoredFileMeta {
	size, _ := strconv.ParseInt(header.Get("Content-Length"), 10, 64)
	lastModified := time.Time{}
	if raw := header.Get("Last-Modified"); raw != "" {
		if parsed, err := http.ParseTime(raw); err == nil {
			lastModified = parsed
		}
	}
	contentType := header.Get("Content-Type")
	if contentType == "" {
		contentType = typeutil.GenericContentType
	}
	return files.StoredFileMeta{
		Key:          key,
		Size:         size,
		ContentType:  contentType,
		LastModified: lastModified,
		ETag:         strings.Trim(header.Get("ETag"), `"`),
	}
}

func statusError(status int, message string) *files.Error {
	code := files.ErrProvider
	if status == http.StatusNotFound {
		code = files.ErrNotFound
	} else if status == http.StatusUnauthorized || status == http.StatusForbidden {
		code = files.ErrUnauthorized
	} else if status == http.StatusConflict {
		code = files.ErrConflict
	}
	if message == "" {
		message = http.StatusText(status)
	}
	return files.NewError(code, message, nil)
}

func mapUploadThingError(err error) *files.Error {
	if err == nil {
		return nil
	}
	return files.WrapError(err, files.ErrProvider)
}

func basename(key string) string {
	base := path.Base(key)
	if base == "." || base == "/" {
		return key
	}
	return base
}

func escapeQuotes(value string) string {
	return strings.ReplaceAll(value, `"`, `\"`)
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

func derefInt64(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}
