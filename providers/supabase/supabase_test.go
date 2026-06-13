package supabase

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	files "github.com/cersho/gofiles-sdk"
)

func jsonReply(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if value != nil {
		if err := json.NewEncoder(w).Encode(value); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNewValidatesAndNormalizesConfig(t *testing.T) {
	t.Setenv("SUPABASE_URL", "")
	t.Setenv("NEXT_PUBLIC_SUPABASE_URL", "")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "")
	t.Setenv("SUPABASE_KEY", "")
	t.Setenv("NEXT_PUBLIC_SUPABASE_ANON_KEY", "")

	if _, err := New(Options{}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("missing bucket error = %#v", err)
	}
	if _, err := New(Options{Bucket: "uploads"}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("missing credentials error = %#v", err)
	}
	adapter, err := New(Options{Bucket: "uploads", URL: "https://abc.supabase.co///", Key: "header.payload.signature"})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.storageURL != "https://abc.supabase.co/storage/v1" {
		t.Fatalf("storage url = %q", adapter.storageURL)
	}
	adapter, err = New(Options{Bucket: "uploads", URL: "https://abc.supabase.co/storage/v1", Key: "header.payload.signature"})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.storageURL != "https://abc.supabase.co/storage/v1" {
		t.Fatalf("storage url with suffix = %q", adapter.storageURL)
	}
}

func TestCoreHTTPContracts(t *testing.T) {
	var uploadedBody string
	var uploadedMetadata map[string]string
	var removed []string
	var copied copyRequest
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("apikey") != "header.payload.signature" || r.Header.Get("Authorization") != "Bearer header.payload.signature" {
			t.Fatalf("missing auth headers: %#v", r.Header)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/storage/v1/object/uploads/a.txt":
			if r.Header.Get("x-upsert") != "true" || r.Header.Get("Content-Type") != "text/plain" || r.Header.Get("Cache-Control") != "public, max-age=60" {
				t.Fatalf("upload headers = %#v", r.Header)
			}
			rawMeta, err := base64.StdEncoding.DecodeString(r.Header.Get("x-metadata"))
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(rawMeta, &uploadedMetadata); err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			uploadedBody = string(data)
			jsonReply(t, w, http.StatusOK, map[string]string{"path": "a.txt"})
		case r.Method == http.MethodGet && r.URL.Path == "/storage/v1/object/uploads/a.txt":
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", "5")
			w.Header().Set("ETag", `"etag-body"`)
			w.Header().Set("Last-Modified", "Tue, 02 Jan 2024 03:04:05 GMT")
			_, _ = w.Write([]byte("hello"))
		case r.Method == http.MethodGet && r.URL.Path == "/storage/v1/object/info/uploads/a.txt":
			jsonReply(t, w, http.StatusOK, map[string]any{
				"contentType":  "text/plain",
				"etag":         `"etag-info"`,
				"lastModified": "2024-01-02T03:04:05Z",
				"metadata":     map[string]any{"author": "me", "count": 2},
				"size":         5,
			})
		case r.Method == http.MethodGet && r.URL.Path == "/storage/v1/object/info/uploads/missing.txt":
			jsonReply(t, w, http.StatusNotFound, map[string]string{"statusCode": "NotFound", "message": "missing"})
		case r.Method == http.MethodDelete && r.URL.Path == "/storage/v1/object/uploads":
			var body deleteRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			removed = body.Prefixes
			jsonReply(t, w, http.StatusOK, []any{})
		case r.Method == http.MethodPost && r.URL.Path == "/storage/v1/object/copy":
			if err := json.NewDecoder(r.Body).Decode(&copied); err != nil {
				t.Fatal(err)
			}
			jsonReply(t, w, http.StatusOK, map[string]string{"Key": "b.txt"})
		case r.Method == http.MethodPost && r.URL.Path == "/storage/v1/object/list-v2/uploads":
			var body listRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Prefix != "dir/" || body.Cursor != "cur-1" || body.Limit != 10 || !body.WithDelimiter {
				t.Fatalf("list body = %#v", body)
			}
			jsonReply(t, w, http.StatusOK, listResponse{
				Folders:    []listFolder{{Key: "dir/nested/", Name: "nested"}},
				HasNext:    true,
				NextCursor: "cur-2",
				Objects: []listObject{{
					Key:  "dir/a.txt",
					Name: "a.txt",
					Metadata: listMetadata{
						ETag:         `"etag-list"`,
						Size:         1,
						MimeType:     "text/plain",
						LastModified: "2024-01-02T03:04:05Z",
					},
				}},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	adapter, err := New(Options{Bucket: "uploads", URL: server.URL, Key: "header.payload.signature"})
	if err != nil {
		t.Fatal(err)
	}
	client := files.MustNew(files.Options{Adapter: adapter})
	var progress []files.UploadProgress
	upload, err := client.Upload(context.Background(), "a.txt", files.StringBody("hello"), files.UploadOptions{
		CacheControl: "public, max-age=60",
		ContentType:  "text/plain",
		Metadata:     map[string]string{"author": "me"},
		OnProgress: func(p files.UploadProgress) {
			progress = append(progress, p)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if uploadedBody != "hello" || uploadedMetadata["author"] != "me" || upload.Size != 5 {
		t.Fatalf("upload=%#v body=%q metadata=%#v", upload, uploadedBody, uploadedMetadata)
	}
	if len(progress) < 2 || progress[0].Loaded != 0 || progress[len(progress)-1].Loaded != 5 {
		t.Fatalf("progress = %#v", progress)
	}

	got, err := client.Download(context.Background(), "a.txt", files.DownloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	text, err := got.Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello" || got.ETag != "etag-body" || got.Size != 5 {
		t.Fatalf("download = %#v text=%q", got, text)
	}
	end := int64(1)
	if _, err := client.Download(context.Background(), "a.txt", files.DownloadOptions{Range: &files.ByteRange{Start: 0, End: &end}}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("range error = %#v", err)
	}

	head, err := client.Head(context.Background(), "a.txt", files.OperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if head.ETag != "etag-info" || head.Metadata["count"] != "2" {
		t.Fatalf("head = %#v", head)
	}
	exists, err := client.Exists(context.Background(), "missing.txt", files.OperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("missing key exists")
	}
	list, err := client.List(context.Background(), files.ListOptions{Prefix: "dir/", Cursor: "cur-1", Limit: 10, Delimiter: "/"})
	if err != nil {
		t.Fatal(err)
	}
	if list.Cursor != "cur-2" || len(list.Items) != 1 || list.Items[0].Key != "dir/a.txt" || len(list.Prefixes) != 1 || list.Prefixes[0] != "dir/nested/" {
		t.Fatalf("list = %#v", list)
	}
	if _, err := client.List(context.Background(), files.ListOptions{Delimiter: "-"}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("delimiter error = %#v", err)
	}
	if err := client.Copy(context.Background(), "a.txt", "b.txt", files.OperationOptions{}); err != nil {
		t.Fatal(err)
	}
	if copied.BucketID != "uploads" || copied.SourceKey != "a.txt" || copied.DestinationKey != "b.txt" {
		t.Fatalf("copy = %#v", copied)
	}
	deleted, err := client.DeleteMany(context.Background(), []string{"a.txt", "b.txt"}, files.DeleteManyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(deleted.Deleted, ",") != "a.txt,b.txt" || strings.Join(removed, ",") != "a.txt,b.txt" {
		t.Fatalf("deleted=%#v removed=%#v", deleted, removed)
	}
}

func TestAuthHeadersSupportNewAPIKeysAndBearerOverride(t *testing.T) {
	var seen []http.Header
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Clone())
		jsonReply(t, w, http.StatusOK, listResponse{})
	}))
	defer server.Close()

	secretAdapter, err := New(Options{Bucket: "uploads", URL: server.URL, Key: "sb_secret_example"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := secretAdapter.List(context.Background(), files.ListOptions{}); err != nil {
		t.Fatal(err)
	}
	if seen[0].Get("apikey") != "sb_secret_example" {
		t.Fatalf("apikey = %q", seen[0].Get("apikey"))
	}
	if got := seen[0].Get("Authorization"); got != "" {
		t.Fatalf("secret key Authorization = %q, want empty", got)
	}

	userAdapter, err := New(Options{
		Bucket:      "uploads",
		URL:         server.URL,
		Key:         "sb_publishable_example",
		BearerToken: "user.jwt.token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := userAdapter.List(context.Background(), files.ListOptions{}); err != nil {
		t.Fatal(err)
	}
	if seen[1].Get("apikey") != "sb_publishable_example" || seen[1].Get("Authorization") != "Bearer user.jwt.token" {
		t.Fatalf("override headers = %#v", seen[1])
	}
}

func TestURLAndSignedUploadContracts(t *testing.T) {
	var signedBody signedURLRequest
	var signedUploadBody map[string]bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/storage/v1/object/sign/uploads/a.txt":
			if err := json.NewDecoder(r.Body).Decode(&signedBody); err != nil {
				t.Fatal(err)
			}
			jsonReply(t, w, http.StatusOK, map[string]string{"signedURL": "/object/sign/uploads/a.txt?token=read"})
		case r.Method == http.MethodPost && r.URL.Path == "/storage/v1/object/upload/sign/uploads/a.txt":
			if err := json.NewDecoder(r.Body).Decode(&signedUploadBody); err != nil {
				t.Fatal(err)
			}
			jsonReply(t, w, http.StatusOK, map[string]string{"signedURL": "/object/upload/sign/uploads/a.txt?token=write"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	publicAdapter, err := New(Options{Bucket: "uploads", URL: server.URL, Key: "sb_secret_123", Public: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := publicAdapter.URL(context.Background(), "dir/a b.txt", files.URLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got != server.URL+"/storage/v1/object/public/uploads/dir/a%20b.txt" {
		t.Fatalf("public url = %q", got)
	}
	cdnAdapter, err := New(Options{Bucket: "uploads", URL: server.URL, Key: "sb_secret_123", PublicBaseURL: "https://cdn.test/base"})
	if err != nil {
		t.Fatal(err)
	}
	got, err = cdnAdapter.URL(context.Background(), "dir/a b.txt", files.URLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://cdn.test/base/dir/a%20b.txt" {
		t.Fatalf("cdn url = %q", got)
	}
	signed, err := cdnAdapter.URL(context.Background(), "a.txt", files.URLOptions{
		ExpiresIn:                  2 * time.Minute,
		ResponseContentDisposition: `attachment; filename="a.txt"`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if signed != server.URL+"/storage/v1/object/sign/uploads/a.txt?token=read" || signedBody.ExpiresIn != 120 || signedBody.Download != "a.txt" {
		t.Fatalf("signed=%q body=%#v", signed, signedBody)
	}
	if _, err := cdnAdapter.URL(context.Background(), "a.txt", files.URLOptions{ResponseContentDisposition: "inline"}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("inline disposition error = %#v", err)
	}
	upload, err := cdnAdapter.SignedUploadURL(context.Background(), "a.txt", files.SignedUploadOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	if upload.Method != http.MethodPut || upload.URL != server.URL+"/storage/v1/object/upload/sign/uploads/a.txt?token=write" || upload.Headers["x-upsert"] != "true" || upload.Headers["Content-Type"] != "text/plain" || !signedUploadBody["upsert"] {
		t.Fatalf("signed upload = %#v body=%#v", upload, signedUploadBody)
	}
	maxSize := int64(10)
	if _, err := cdnAdapter.SignedUploadURL(context.Background(), "a.txt", files.SignedUploadOptions{MaxSize: &maxSize}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("max size error = %#v", err)
	}
}

func TestResumableUploadTUS(t *testing.T) {
	const sixMiB = 6 * 1024 * 1024
	var methods []string
	var uploadMetadata string
	var offsets []string
	headCount := 0
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Tus-Resumable") != "1.0.0" || r.Header.Get("apikey") != "header.payload.signature" {
			t.Fatalf("bad tus headers = %#v", r.Header)
		}
		methods = append(methods, r.Method)
		switch r.Method {
		case http.MethodPost:
			if r.URL.Path != "/storage/v1/upload/resumable" {
				t.Fatalf("post path = %s", r.URL.Path)
			}
			uploadMetadata = r.Header.Get("Upload-Metadata")
			w.Header().Set("Location", server.URL+"/storage/v1/upload/resumable/session-1")
			w.WriteHeader(http.StatusCreated)
		case http.MethodHead:
			if headCount == 0 {
				w.Header().Set("Upload-Offset", "0")
			} else {
				w.Header().Set("Upload-Offset", strconvItoa(sixMiB))
			}
			headCount++
			w.WriteHeader(http.StatusOK)
		case http.MethodPatch:
			offsets = append(offsets, r.Header.Get("Upload-Offset"))
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			next := sixMiB + len(data)
			w.Header().Set("Upload-Offset", strconvItoa(next))
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method = %s", r.Method)
		}
	}))
	defer server.Close()

	adapter, err := New(Options{Bucket: "uploads", URL: server.URL, Key: "header.payload.signature"})
	if err != nil {
		t.Fatal(err)
	}
	filesClient := files.MustNew(files.Options{Adapter: adapter})
	control := files.NewUploadControl()
	data := bytes.Repeat([]byte("a"), sixMiB+10)
	out, err := filesClient.Upload(context.Background(), "large.bin", files.BytesBody(data), files.UploadOptions{
		ContentType: "application/octet-stream",
		Control:     control,
		Multipart:   &files.MultipartOptions{PartSize: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Size != int64(len(data)) || control.Status() != files.UploadControlCompleted {
		t.Fatalf("out=%#v status=%s", out, control.Status())
	}
	session, ok := control.Session()
	if !ok || session.Provider != "supabase" || session.TempPath == "" || session.PartSize != minTusPartSize {
		t.Fatalf("session=%#v ok=%v", session, ok)
	}
	if !strings.Contains(uploadMetadata, "bucketName "+base64.StdEncoding.EncodeToString([]byte("uploads"))) || !strings.Contains(uploadMetadata, "objectName "+base64.StdEncoding.EncodeToString([]byte("large.bin"))) {
		t.Fatalf("upload metadata = %q", uploadMetadata)
	}
	if strings.Join(offsets, ",") != "0,"+strconvItoa(sixMiB) {
		t.Fatalf("offsets = %#v", offsets)
	}

	resumeControl := files.UploadControlFrom(files.ResumableSession{
		Provider:    "supabase",
		Key:         "large.bin",
		Bucket:      "uploads",
		TempPath:    server.URL + "/storage/v1/upload/resumable/session-1",
		PartSize:    minTusPartSize,
		ContentType: "application/octet-stream",
	})
	out, err = filesClient.Upload(context.Background(), "large.bin", files.BytesBody(data), files.UploadOptions{
		ContentType: "application/octet-stream",
		Control:     resumeControl,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Size != 10 {
		t.Fatalf("resumed size = %d, want remaining part size 10", out.Size)
	}
	driver, err := adapter.ResumableUpload(context.Background(), "large.bin", files.ResumableUploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.Adopt(context.Background(), files.ResumableSession{Provider: "s3", Key: "large.bin"}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("adopt error = %#v", err)
	}
	if err := driver.Abort(context.Background()); err != nil {
		t.Fatal(err)
	}
	_ = methods
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		body   map[string]string
		code   files.ErrorCode
	}{
		{http.StatusNotFound, map[string]string{"statusCode": "NotFound", "message": "missing"}, files.ErrNotFound},
		{http.StatusUnauthorized, map[string]string{"message": "unauthorized"}, files.ErrUnauthorized},
		{http.StatusConflict, map[string]string{"statusCode": "Duplicate", "message": "exists"}, files.ErrConflict},
		{http.StatusInternalServerError, map[string]string{"message": "oops"}, files.ErrProvider},
	}
	for _, tc := range cases {
		data, _ := json.Marshal(tc.body)
		if got := statusError(tc.status, data); got.Code != tc.code {
			t.Fatalf("status %d got %s want %s", tc.status, got.Code, tc.code)
		}
	}
}

func strconvItoa(value int) string {
	return strconv.FormatInt(int64(value), 10)
}
