package vercelblob

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonReply(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatal(err)
	}
}

func response(status int, body string, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	if header.Get("Content-Length") == "" {
		header.Set("Content-Length", strconv.Itoa(len(body)))
	}
	return &http.Response{
		StatusCode: status,
		Status:     strconv.Itoa(status) + " " + http.StatusText(status),
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestNewAuthResolutionAndURLFastPath(t *testing.T) {
	t.Setenv("BLOB_READ_WRITE_TOKEN", "")
	t.Setenv("VERCEL_OIDC_TOKEN", "")
	t.Setenv("BLOB_STORE_ID", "")

	if _, err := New(Options{}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("missing credentials error = %#v", err)
	}

	adapter, err := New(Options{Token: "vercel_blob_rw_abc123store_random"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.URL(context.Background(), "my file?q#frag", files.URLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://abc123store.public.blob.vercel-storage.com/my%20file%3Fq%23frag"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}

	adapter, err = New(Options{StoreID: "store_abc123store", Token: "plain-token"})
	if err != nil {
		t.Fatal(err)
	}
	got, err = adapter.URL(context.Background(), "a/b.txt", files.URLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want = "https://abc123store.public.blob.vercel-storage.com/a/b.txt"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}

	t.Setenv("BLOB_READ_WRITE_TOKEN", "rw-token")
	t.Setenv("VERCEL_OIDC_TOKEN", "oidc-token")
	t.Setenv("BLOB_STORE_ID", "store_abc123store")
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer oidc-token" {
			t.Fatalf("authorization = %q", req.Header.Get("Authorization"))
		}
		if req.Header.Get("x-vercel-blob-store-id") != "abc123store" {
			t.Fatalf("store id = %q", req.Header.Get("x-vercel-blob-store-id"))
		}
		return response(http.StatusOK, `{"pathname":"a.txt","size":1,"contentType":"text/plain","uploadedAt":"2024-01-01T00:00:00Z","url":"https://blob.test/a.txt","etag":"e"}`, nil), nil
	})}
	adapter, err = New(Options{APIURL: "https://api.test", HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Head(context.Background(), "a.txt", files.OperationOptions{}); err != nil {
		t.Fatal(err)
	}
}

func TestPublicAdapterCoreHTTPContracts(t *testing.T) {
	var uploadedBody string
	var deleteURLs []string
	var copied bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/" && r.Method == http.MethodPut && r.URL.Query().Get("pathname") == "a.txt":
			if r.Header.Get("Authorization") != "Bearer vercel_blob_rw_abc123store_random" {
				t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
			}
			if r.Header.Get("x-api-version") != apiVersion {
				t.Fatalf("api version = %q", r.Header.Get("x-api-version"))
			}
			if r.Header.Get("x-vercel-blob-access") != "public" ||
				r.Header.Get("x-add-random-suffix") != "0" ||
				r.Header.Get("x-allow-overwrite") != "1" ||
				r.Header.Get("x-content-type") != "text/plain" ||
				r.Header.Get("x-cache-control-max-age") != "60" ||
				r.Header.Get("x-content-length") != "5" {
				t.Fatalf("unexpected upload headers: %#v", r.Header)
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			uploadedBody = string(data)
			jsonReply(t, w, http.StatusOK, putBlobResponse{
				URL:         serverURL(r, "/public/a.txt"),
				DownloadURL: serverURL(r, "/public/a.txt?download=1"),
				Pathname:    "a.txt",
				ContentType: "text/plain",
				ETag:        "etag-put",
			})
		case r.URL.Path == "/" && r.Method == http.MethodGet && r.URL.Query().Get("url") == "a.txt":
			jsonReply(t, w, http.StatusOK, headResponse{
				URL:         serverURL(r, "/public/a.txt"),
				DownloadURL: serverURL(r, "/public/a.txt?download=1"),
				Pathname:    "a.txt",
				ContentType: "text/plain",
				Size:        5,
				UploadedAt:  "2024-01-02T03:04:05Z",
				ETag:        "etag-head",
			})
		case r.URL.Path == "/public/a.txt" && r.Method == http.MethodGet:
			if got := r.Header.Get("Range"); got != "" && got != "bytes=1-3" {
				t.Fatalf("range = %q", got)
			}
			if r.Header.Get("Range") == "bytes=1-3" {
				w.Header().Set("Content-Type", "text/plain")
				w.Header().Set("Content-Length", "3")
				w.WriteHeader(http.StatusPartialContent)
				_, _ = w.Write([]byte("ell"))
				return
			}
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", "5")
			_, _ = w.Write([]byte("hello"))
		case r.URL.Path == "/" && r.Method == http.MethodGet && r.URL.Query().Get("prefix") == "dir/":
			if r.URL.Query().Get("mode") != "folded" || r.URL.Query().Get("limit") != "10" || r.URL.Query().Get("cursor") != "cursor-1" {
				t.Fatalf("list query = %s", r.URL.RawQuery)
			}
			jsonReply(t, w, http.StatusOK, listResponse{
				Blobs: []listBlob{{
					URL:        serverURL(r, "/public/dir/a.txt"),
					Pathname:   "dir/a.txt",
					Size:       1,
					UploadedAt: "2024-01-02T03:04:05Z",
					ETag:       "etag-list",
				}},
				Folders: []string{"dir/nested/"},
				Cursor:  "cursor-2",
				HasMore: true,
			})
		case r.URL.Path == "/public/dir/a.txt" && r.Method == http.MethodGet:
			w.Header().Set("Content-Length", "1")
			_, _ = w.Write([]byte("x"))
		case r.URL.Path == "/delete" && r.Method == http.MethodPost:
			var body deleteRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			deleteURLs = body.URLs
			jsonReply(t, w, http.StatusOK, map[string]bool{"ok": true})
		case r.URL.Path == "/" && r.Method == http.MethodPut && r.URL.Query().Get("pathname") == "b.txt":
			if r.URL.Query().Get("fromUrl") != "a.txt" {
				t.Fatalf("fromUrl = %q", r.URL.Query().Get("fromUrl"))
			}
			copied = true
			jsonReply(t, w, http.StatusOK, putBlobResponse{Pathname: "b.txt", ETag: "etag-copy"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	adapter, err := New(Options{
		Token:      "vercel_blob_rw_abc123store_random",
		APIURL:     server.URL,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	client := files.MustNew(files.Options{Adapter: adapter})
	var progress []files.UploadProgress
	upload, err := client.Upload(context.Background(), "a.txt", files.StringBody("hello"), files.UploadOptions{
		ContentType:  "text/plain",
		CacheControl: "public, max-age=60",
		OnProgress: func(p files.UploadProgress) {
			progress = append(progress, p)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if uploadedBody != "hello" || upload.Key != "a.txt" || upload.Size != 5 || upload.ETag != "etag-put" {
		t.Fatalf("upload/body = %#v / %q", upload, uploadedBody)
	}
	if len(progress) == 0 || progress[0].Loaded != 0 || progress[len(progress)-1].Loaded != 5 {
		t.Fatalf("progress = %#v", progress)
	}

	end := int64(3)
	download, err := client.Download(context.Background(), "a.txt", files.DownloadOptions{Range: &files.ByteRange{Start: 1, End: &end}})
	if err != nil {
		t.Fatal(err)
	}
	text, err := download.Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if text != "ell" || download.Size != 3 {
		t.Fatalf("download = %q size=%d", text, download.Size)
	}

	list, err := client.List(context.Background(), files.ListOptions{Prefix: "dir/", Delimiter: "/", Limit: 10, Cursor: "cursor-1"})
	if err != nil {
		t.Fatal(err)
	}
	if list.Cursor != "cursor-2" || len(list.Prefixes) != 1 || list.Prefixes[0] != "dir/nested/" || len(list.Items) != 1 {
		t.Fatalf("list = %#v", list)
	}
	itemText, err := list.Items[0].Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if itemText != "x" {
		t.Fatalf("list item body = %q", itemText)
	}

	if err := client.Copy(context.Background(), "a.txt", "b.txt", files.OperationOptions{}); err != nil {
		t.Fatal(err)
	}
	if !copied {
		t.Fatal("copy was not called")
	}
	deleted, err := client.DeleteMany(context.Background(), []string{"a.txt", "b.txt"}, files.DeleteManyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted.Deleted) != 2 || strings.Join(deleteURLs, ",") != "a.txt,b.txt" {
		t.Fatalf("deleted = %#v urls=%#v", deleted, deleteURLs)
	}
	if _, err := client.List(context.Background(), files.ListOptions{Delimiter: "-"}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("delimiter error = %#v", err)
	}
	if _, err := client.URL(context.Background(), "a.txt", files.URLOptions{ResponseContentDisposition: "attachment"}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("url content-disposition error = %#v", err)
	}
	if _, err := client.SignedUploadURL(context.Background(), "a.txt", files.SignedUploadOptions{}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("signed upload error = %#v", err)
	}
}

func TestPrivateAdapterUsesAuthenticatedBlobFetch(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "api.test" && req.Method == http.MethodGet:
			return response(http.StatusOK, `{"pathname":"secret.txt","size":6,"contentType":"text/plain","uploadedAt":"2024-01-01T00:00:00Z","url":"https://abc123store.private.blob.vercel-storage.com/secret.txt","etag":"e"}`, nil), nil
		case req.URL.Host == "abc123store.private.blob.vercel-storage.com" && req.Method == http.MethodGet:
			if req.Header.Get("Authorization") != "Bearer rw-token" {
				t.Fatalf("private fetch authorization = %q", req.Header.Get("Authorization"))
			}
			header := http.Header{"Content-Type": []string{"text/plain"}}
			return response(http.StatusOK, "secret", header), nil
		default:
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
			return nil, nil
		}
	})}
	adapter, err := New(Options{
		Token:      "rw-token",
		StoreID:    "abc123store",
		Access:     AccessPrivate,
		APIURL:     "https://api.test",
		HTTPClient: client,
	})
	if err != nil {
		t.Fatal(err)
	}
	filesClient := files.MustNew(files.Options{Adapter: adapter})
	if adapter.Capabilities().RangeRead {
		t.Fatal("private adapter must not advertise range reads")
	}
	stored, err := filesClient.Download(context.Background(), "secret.txt", files.DownloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	text, err := stored.Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if text != "secret" {
		t.Fatalf("private body = %q", text)
	}
	if _, err := filesClient.URL(context.Background(), "secret.txt", files.URLOptions{}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("private url error = %#v", err)
	}
	end := int64(1)
	if _, err := filesClient.Download(context.Background(), "secret.txt", files.DownloadOptions{Range: &files.ByteRange{Start: 0, End: &end}}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("private range error = %#v", err)
	}
}

func TestErrorsAndURLFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("url") == "missing.txt" {
			jsonReply(t, w, http.StatusNotFound, map[string]any{"error": map[string]string{"code": "not_found", "message": "missing"}})
			return
		}
		jsonReply(t, w, http.StatusOK, headResponse{Pathname: "randomized.txt", URL: "https://blob.test/randomized.txt", Size: 1})
	}))
	defer server.Close()

	adapter, err := New(Options{
		Token:           "vercel_blob_rw_abc123store_random",
		AddRandomSuffix: true,
		APIURL:          server.URL,
		HTTPClient:      server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	exists, err := adapter.Exists(context.Background(), "missing.txt", files.OperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("missing blob exists")
	}
	got, err := adapter.URL(context.Background(), "a.txt", files.URLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://blob.test/randomized.txt" {
		t.Fatalf("fallback url = %q", got)
	}
}

func TestResumableUploadAndInvalidAdopt(t *testing.T) {
	var uploadedParts []int
	var completed []completedPart
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/mpu" || r.Method != http.MethodPost || r.URL.Query().Get("pathname") != "large.bin" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		switch r.Header.Get("x-mpu-action") {
		case "create":
			if r.Header.Get("x-vercel-blob-access") != "public" || r.Header.Get("x-content-type") != "application/octet-stream" {
				t.Fatalf("create headers = %#v", r.Header)
			}
			jsonReply(t, w, http.StatusOK, createMultipartResponse{Key: "storage-key", UploadID: "upload-1"})
		case "upload":
			if r.Header.Get("x-mpu-key") != url.PathEscape("storage-key") || r.Header.Get("x-mpu-upload-id") != "upload-1" {
				t.Fatalf("upload headers = %#v", r.Header)
			}
			partNumber, err := strconv.Atoi(r.Header.Get("x-mpu-part-number"))
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			if partNumber == 1 && int64(len(data)) != minMultipartPartSize {
				t.Fatalf("part 1 size = %d", len(data))
			}
			uploadedParts = append(uploadedParts, partNumber)
			jsonReply(t, w, http.StatusOK, uploadPartResponse{ETag: "etag-" + strconv.Itoa(partNumber)})
		case "complete":
			if err := json.NewDecoder(r.Body).Decode(&completed); err != nil {
				t.Fatal(err)
			}
			jsonReply(t, w, http.StatusOK, putBlobResponse{Pathname: "large.bin", ContentType: "application/octet-stream", ETag: "etag-final"})
		default:
			t.Fatalf("mpu action = %q", r.Header.Get("x-mpu-action"))
		}
	}))
	defer server.Close()

	adapter, err := New(Options{
		Token:      "vercel_blob_rw_abc123store_random",
		APIURL:     server.URL,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	driver, err := adapter.ResumableUpload(context.Background(), "large.bin", files.ResumableUploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.Adopt(context.Background(), files.ResumableSession{Provider: "s3", Key: "large.bin"}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("invalid adopt error = %#v", err)
	}

	client := files.MustNew(files.Options{Adapter: adapter})
	control := files.NewUploadControl()
	data := bytes.Repeat([]byte("a"), int(minMultipartPartSize)+3)
	out, err := client.Upload(context.Background(), "large.bin", files.BytesBody(data), files.UploadOptions{
		ContentType: "application/octet-stream",
		Control:     control,
		Multipart:   &files.MultipartOptions{PartSize: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Size != int64(len(data)) || out.ETag != "etag-final" {
		t.Fatalf("resumable result = %#v", out)
	}
	if len(uploadedParts) != 2 || uploadedParts[0] != 1 || uploadedParts[1] != 2 {
		t.Fatalf("uploaded parts = %#v", uploadedParts)
	}
	if len(completed) != 2 || completed[0].ETag != "etag-1" || completed[1].PartNumber != 2 {
		t.Fatalf("completed = %#v", completed)
	}
	session, ok := control.Session()
	if !ok || session.Provider != "vercel-blob" || session.TempPath != "storage-key" || session.PartSize != minMultipartPartSize {
		t.Fatalf("session = %#v ok=%v", session, ok)
	}
}

func serverURL(r *http.Request, path string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host + path
}
