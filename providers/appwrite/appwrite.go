package appwrite

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	files "github.com/cersho/gofiles-sdk"
	"github.com/cersho/gofiles-sdk/internal/envutil"
	"github.com/cersho/gofiles-sdk/internal/typeutil"
)

const (
	defaultEndpoint  = "https://cloud.appwrite.io/v1"
	defaultListLimit = int32(100)
)

var appwriteFileIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,35}$`)

type Options struct {
	Bucket     string
	Endpoint   string
	ProjectID  string
	Key        string
	Public     bool
	HTTPClient *http.Client
}

type Adapter struct {
	bucket     string
	endpoint   string
	projectID  string
	key        string
	public     bool
	httpClient *http.Client
}

func New(opts Options) (*Adapter, error) {
	if opts.Bucket == "" {
		return nil, files.NewError(files.ErrProvider, "appwrite adapter: missing bucket. Pass Bucket.", nil)
	}
	endpoint := envutil.First(opts.Endpoint, os.Getenv("APPWRITE_ENDPOINT"), os.Getenv("NEXT_PUBLIC_APPWRITE_ENDPOINT"), defaultEndpoint)
	projectID := envutil.First(opts.ProjectID, os.Getenv("APPWRITE_PROJECT_ID"), os.Getenv("NEXT_PUBLIC_APPWRITE_PROJECT_ID"))
	key := envutil.First(opts.Key, os.Getenv("APPWRITE_API_KEY"), os.Getenv("APPWRITE_KEY"))
	if projectID == "" {
		return nil, files.NewError(files.ErrProvider, "appwrite adapter: missing projectId. Pass ProjectID or set APPWRITE_PROJECT_ID / NEXT_PUBLIC_APPWRITE_PROJECT_ID.", nil)
	}
	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &Adapter{
		bucket:     opts.Bucket,
		endpoint:   strings.TrimRight(endpoint, "/"),
		projectID:  projectID,
		key:        key,
		public:     opts.Public,
		httpClient: client,
	}, nil
}

func (a *Adapter) Name() string { return "appwrite" }

func (a *Adapter) Raw() any { return a.httpClient }

func (a *Adapter) Bucket() string { return a.bucket }

func (a *Adapter) Capabilities() files.AdapterCapabilities {
	return files.AdapterCapabilities{
		UploadProgress: true,
		Resumable:      true,
		ServerSideCopy: false,
		SignedURL:      files.SignedURLCapability{Supported: false},
	}
}

func (a *Adapter) Upload(ctx context.Context, key string, body files.Body, opts files.UploadOptions) (files.UploadResult, error) {
	if err := validateFileID(key, "key"); err != nil {
		return files.UploadResult{}, err
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
	var decoded appwriteFile
	if err := a.uploadReader(ctx, key, key, contentType, reader, -1, nil, &decoded); err != nil {
		return files.UploadResult{}, err
	}
	if opts.OnProgress != nil {
		opts.OnProgress(files.UploadProgress{Loaded: decoded.SizeOriginal, Total: decoded.SizeOriginal, Known: true})
	}
	return uploadResult(key, contentType, decoded), nil
}

func (a *Adapter) Download(ctx context.Context, key string, _ files.DownloadOptions) (files.StoredFile, error) {
	stat, err := a.getFile(ctx, key)
	if err != nil {
		return files.StoredFile{}, err
	}
	body, header, err := a.fetchDownload(ctx, key)
	if err != nil {
		return files.StoredFile{}, err
	}
	meta := metaFromFile(key, stat)
	if got := contentLength(header); got > 0 {
		meta.Size = got
	}
	if got := header.Get("Content-Type"); got != "" {
		meta.ContentType = got
	}
	return files.NewStoredFile(meta, func(context.Context) (io.ReadCloser, error) { return body, nil }), nil
}

func (a *Adapter) Head(ctx context.Context, key string, _ files.OperationOptions) (files.StoredFile, error) {
	stat, err := a.getFile(ctx, key)
	if err != nil {
		return files.StoredFile{}, err
	}
	return files.NewStoredFile(metaFromFile(key, stat), func(ctx context.Context) (io.ReadCloser, error) {
		body, _, err := a.fetchDownload(ctx, key)
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
	if err := validateFileID(key, "key"); err != nil {
		return err
	}
	_, err := a.request(ctx, http.MethodDelete, "/storage/buckets/"+url.PathEscape(a.bucket)+"/files/"+url.PathEscape(key), nil, nil, -1, nil)
	return err
}

func (a *Adapter) Copy(ctx context.Context, from string, to string, _ files.OperationOptions) error {
	if err := validateFileID(to, "copy destination"); err != nil {
		return err
	}
	body, _, err := a.fetchDownload(ctx, from)
	if err != nil {
		return err
	}
	defer body.Close()
	return a.uploadReader(ctx, to, to, "", body, -1, nil, nil)
}

func (a *Adapter) List(ctx context.Context, opts files.ListOptions) (files.ListResult, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = defaultListLimit
	}
	params := url.Values{}
	params.Add("queries[]", queryLimit(limit))
	if opts.Prefix != "" {
		params.Add("queries[]", queryStartsWith("name", opts.Prefix))
	}
	if opts.Cursor != "" {
		params.Add("queries[]", queryCursorAfter(opts.Cursor))
	}
	var decoded listResponse
	if _, err := a.request(ctx, http.MethodGet, "/storage/buckets/"+url.PathEscape(a.bucket)+"/files?"+params.Encode(), nil, nil, -1, &decoded); err != nil {
		return files.ListResult{}, err
	}
	out := files.ListResult{}
	for _, item := range decoded.Files {
		file := item
		key := file.ID
		out.Items = append(out.Items, files.NewStoredFile(metaFromFile(key, file), func(ctx context.Context) (io.ReadCloser, error) {
			body, _, err := a.fetchDownload(ctx, key)
			return body, err
		}))
	}
	if int32(len(decoded.Files)) == limit && len(decoded.Files) > 0 {
		out.Cursor = decoded.Files[len(decoded.Files)-1].ID
	}
	return out, nil
}

func (a *Adapter) URL(_ context.Context, key string, opts files.URLOptions) (string, error) {
	if opts.ResponseContentDisposition != "" {
		return "", files.NewError(files.ErrProvider, "appwrite: responseContentDisposition is not supported", nil)
	}
	if !a.public {
		return "", files.NewError(files.ErrProvider, "appwrite: url is not supported unless the bucket is public. Set Public: true to return a permanent view URL.", nil)
	}
	if err := validateFileID(key, "key"); err != nil {
		return "", err
	}
	return a.endpoint + "/storage/buckets/" + url.PathEscape(a.bucket) + "/files/" + url.PathEscape(key) + "/view?project=" + url.QueryEscape(a.projectID), nil
}

func (a *Adapter) SignedUploadURL(context.Context, string, files.SignedUploadOptions) (files.SignedUpload, error) {
	return files.SignedUpload{}, files.NewError(files.ErrProvider, "appwrite: signedUploadUrl is not supported. Appwrite has no presigned upload primitive; use JWTs or client SDK uploads.", nil)
}

func (a *Adapter) getFile(ctx context.Context, key string) (appwriteFile, error) {
	if err := validateFileID(key, "key"); err != nil {
		return appwriteFile{}, err
	}
	var decoded appwriteFile
	if _, err := a.request(ctx, http.MethodGet, "/storage/buckets/"+url.PathEscape(a.bucket)+"/files/"+url.PathEscape(key), nil, nil, -1, &decoded); err != nil {
		return appwriteFile{}, err
	}
	return decoded, nil
}

func (a *Adapter) fetchDownload(ctx context.Context, key string) (io.ReadCloser, http.Header, error) {
	if err := validateFileID(key, "key"); err != nil {
		return nil, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.endpoint+"/storage/buckets/"+url.PathEscape(a.bucket)+"/files/"+url.PathEscape(key)+"/download", nil)
	if err != nil {
		return nil, nil, files.NewError(files.ErrProvider, err.Error(), err)
	}
	a.addHeaders(req)
	res, err := a.httpClient.Do(req)
	if err != nil {
		return nil, nil, mapAppwriteError(err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		data, _ := io.ReadAll(res.Body)
		res.Body.Close()
		return nil, nil, statusError(res.StatusCode, data)
	}
	return res.Body, res.Header, nil
}

func (a *Adapter) uploadReader(ctx context.Context, fileID string, filename string, contentType string, reader io.Reader, contentLength int64, extraHeaders http.Header, out any) error {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		defer pw.Close()
		defer writer.Close()
		if err := writer.WriteField("fileId", fileID); err != nil {
			_ = pw.CloseWithError(err)
			return
		}
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="file"; filename="`+escapeQuotes(filename)+`"`)
		if contentType != "" {
			header.Set("Content-Type", contentType)
		}
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
	headers := http.Header{}
	headers.Set("Content-Type", writer.FormDataContentType())
	for key, values := range extraHeaders {
		for _, value := range values {
			headers.Add(key, value)
		}
	}
	_, err := a.request(ctx, http.MethodPost, "/storage/buckets/"+url.PathEscape(a.bucket)+"/files", headers, pr, contentLength, out)
	return err
}

func (a *Adapter) request(ctx context.Context, method string, pathAndQuery string, headers http.Header, body io.Reader, contentLength int64, out any) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, a.endpoint+pathAndQuery, body)
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
	a.addHeaders(req)
	res, err := a.httpClient.Do(req)
	if err != nil {
		return nil, mapAppwriteError(err)
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

func (a *Adapter) addHeaders(req *http.Request) {
	req.Header.Set("X-Appwrite-Project", a.projectID)
	if a.key != "" {
		req.Header.Set("X-Appwrite-Key", a.key)
	}
}

type appwriteFile struct {
	ID           string `json:"$id"`
	MimeType     string `json:"mimeType"`
	SizeOriginal int64  `json:"sizeOriginal"`
}

type listResponse struct {
	Files []appwriteFile `json:"files"`
	Total int            `json:"total"`
}

type appwriteErrorBody struct {
	Message string `json:"message"`
	Code    any    `json:"code"`
	Type    string `json:"type"`
}

func statusError(status int, data []byte) *files.Error {
	message := strings.TrimSpace(string(data))
	var decoded appwriteErrorBody
	if len(data) > 0 && json.Unmarshal(data, &decoded) == nil && decoded.Message != "" {
		message = decoded.Message
	}
	if message == "" {
		message = http.StatusText(status)
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

func mapAppwriteError(err error) *files.Error {
	if err == nil {
		return nil
	}
	var fe *files.Error
	if errors.As(err, &fe) {
		return fe
	}
	return files.WrapError(err, files.ErrProvider)
}

func validateFileID(key string, label string) error {
	if !appwriteFileIDPattern.MatchString(key) {
		return files.NewError(files.ErrProvider, fmt.Sprintf("appwrite: %s %q is not a valid Appwrite file ID; must be 1-36 chars, start with [a-zA-Z0-9], and use only [a-zA-Z0-9._-] (no slashes).", label, key), nil)
	}
	return nil
}

func metaFromFile(key string, file appwriteFile) files.StoredFileMeta {
	if file.ID != "" {
		key = file.ID
	}
	return files.StoredFileMeta{
		Key:         key,
		Size:        file.SizeOriginal,
		ContentType: typeutil.EffectiveContentType(file.MimeType),
	}
}

func uploadResult(key string, fallbackContentType string, file appwriteFile) files.UploadResult {
	if file.ID != "" {
		key = file.ID
	}
	return files.UploadResult{
		Key:         key,
		Size:        file.SizeOriginal,
		ContentType: typeutil.EffectiveContentType(file.MimeType, fallbackContentType),
	}
}

func contentLength(header http.Header) int64 {
	size, _ := strconv.ParseInt(header.Get("Content-Length"), 10, 64)
	return size
}

func queryLimit(limit int32) string {
	return query("limit", "", []any{limit})
}

func queryCursorAfter(cursor string) string {
	return query("cursorAfter", "", []any{cursor})
}

func queryStartsWith(attr string, value string) string {
	return query("startsWith", attr, []any{value})
}

func query(method string, attribute string, values []any) string {
	payload := struct {
		Method    string `json:"method"`
		Attribute string `json:"attribute,omitempty"`
		Values    []any  `json:"values,omitempty"`
	}{
		Method: method,
		Values: values,
	}
	if attribute != "" {
		payload.Attribute = attribute
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

func escapeQuotes(value string) string {
	return strings.ReplaceAll(value, `"`, `\"`)
}

func sumParts(parts []files.PartMeta) int64 {
	var size int64
	for _, part := range parts {
		size += part.Size
	}
	return size
}

func readerFromBytes(data []byte) io.Reader {
	return bytes.NewReader(data)
}
