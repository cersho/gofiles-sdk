package appwrite

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	files "github.com/cersho/gofiles-sdk"
)

const (
	testBucket  = "uploads"
	testProject = "proj123"
	testKey     = "api-key"
)

func writeJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if value != nil {
		if err := json.NewEncoder(w).Encode(value); err != nil {
			t.Fatal(err)
		}
	}
}

func readMultipartFile(t *testing.T, r *http.Request) (string, []byte) {
	t.Helper()
	reader, err := r.MultipartReader()
	if err != nil {
		t.Fatal(err)
	}
	fileID := ""
	var data []byte
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(part)
		if err != nil {
			t.Fatal(err)
		}
		switch part.FormName() {
		case "fileId":
			fileID = string(raw)
		case "file":
			data = raw
		}
	}
	return fileID, data
}

func TestNewValidatesAndUsesEnv(t *testing.T) {
	t.Setenv("APPWRITE_ENDPOINT", "")
	t.Setenv("NEXT_PUBLIC_APPWRITE_ENDPOINT", "")
	t.Setenv("APPWRITE_PROJECT_ID", "")
	t.Setenv("NEXT_PUBLIC_APPWRITE_PROJECT_ID", "")
	t.Setenv("APPWRITE_API_KEY", "")
	t.Setenv("APPWRITE_KEY", "")

	if _, err := New(Options{}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("missing bucket error = %#v", err)
	}
	if _, err := New(Options{Bucket: testBucket}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("missing project error = %#v", err)
	}
	t.Setenv("NEXT_PUBLIC_APPWRITE_ENDPOINT", "https://example.test/v1/")
	t.Setenv("NEXT_PUBLIC_APPWRITE_PROJECT_ID", testProject)
	t.Setenv("APPWRITE_KEY", testKey)
	adapter, err := New(Options{Bucket: testBucket})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.endpoint != "https://example.test/v1" || adapter.projectID != testProject || adapter.key != testKey {
		t.Fatalf("adapter config = %#v", adapter)
	}
}

func TestCoreHTTPContracts(t *testing.T) {
	var uploadedBody string
	var copiedBody string
	var deleted string
	var listQueries []string
	downloadCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Appwrite-Project") != testProject || r.Header.Get("X-Appwrite-Key") != testKey {
			t.Fatalf("missing appwrite headers: %#v", r.Header)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/storage/buckets/uploads/files":
			fileID, data := readMultipartFile(t, r)
			if fileID == "a" {
				uploadedBody = string(data)
			}
			if fileID == "b" {
				copiedBody = string(data)
			}
			writeJSON(t, w, http.StatusCreated, map[string]any{"$id": fileID, "mimeType": "text/plain", "sizeOriginal": len(data)})
		case r.Method == http.MethodGet && r.URL.Path == "/storage/buckets/uploads/files/a":
			writeJSON(t, w, http.StatusOK, map[string]any{"$id": "a", "mimeType": "text/plain", "sizeOriginal": 5})
		case r.Method == http.MethodGet && r.URL.Path == "/storage/buckets/uploads/files/missing":
			writeJSON(t, w, http.StatusNotFound, map[string]any{"message": "missing"})
		case r.Method == http.MethodGet && r.URL.Path == "/storage/buckets/uploads/files/a/download":
			downloadCount++
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Length", "5")
			_, _ = w.Write([]byte("hello"))
		case r.Method == http.MethodDelete && r.URL.Path == "/storage/buckets/uploads/files/a":
			deleted = "a"
			writeJSON(t, w, http.StatusNoContent, nil)
		case r.Method == http.MethodGet && r.URL.Path == "/storage/buckets/uploads/files":
			listQueries = r.URL.Query()["queries[]"]
			writeJSON(t, w, http.StatusOK, listResponse{
				Files: []appwriteFile{
					{ID: "a", MimeType: "text/plain", SizeOriginal: 5},
					{ID: "b", MimeType: "image/png", SizeOriginal: 7},
				},
				Total: 2,
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	adapter, err := New(Options{Bucket: testBucket, Endpoint: server.URL, ProjectID: testProject, Key: testKey, Public: true})
	if err != nil {
		t.Fatal(err)
	}
	client := files.MustNew(files.Options{Adapter: adapter})
	out, err := client.Upload(context.Background(), "a", files.StringBody("hello"), files.UploadOptions{ContentType: "text/plain"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Key != "a" || out.Size != 5 || uploadedBody != "hello" {
		t.Fatalf("upload=%#v body=%q", out, uploadedBody)
	}
	if _, err := client.Upload(context.Background(), "has/slash", files.StringBody("x"), files.UploadOptions{}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("invalid key error = %#v", err)
	}
	got, err := client.Download(context.Background(), "a", files.DownloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	text, err := got.Text(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello" || got.Size != 5 || got.ContentType != "text/plain" {
		t.Fatalf("download = %#v text=%q", got, text)
	}
	head, err := client.Head(context.Background(), "a", files.OperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if downloadCount != 1 {
		t.Fatalf("head fetched body early")
	}
	if _, err := head.Text(context.Background()); err != nil {
		t.Fatal(err)
	}
	if downloadCount != 2 {
		t.Fatalf("lazy head body did not fetch")
	}
	exists, err := client.Exists(context.Background(), "missing", files.OperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Fatal("missing file exists")
	}
	list, err := client.List(context.Background(), files.ListOptions{Prefix: "a", Cursor: "cur", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if list.Cursor != "b" || len(list.Items) != 2 {
		t.Fatalf("list = %#v", list)
	}
	joinedQueries := strings.Join(listQueries, "\n")
	if !strings.Contains(joinedQueries, `"method":"limit"`) ||
		!strings.Contains(joinedQueries, `"method":"startsWith"`) ||
		!strings.Contains(joinedQueries, `"column":"name"`) ||
		!strings.Contains(joinedQueries, `"method":"cursorAfter"`) {
		t.Fatalf("list queries = %#v", listQueries)
	}
	if err := client.Copy(context.Background(), "a", "b", files.OperationOptions{}); err != nil {
		t.Fatal(err)
	}
	if copiedBody != "hello" {
		t.Fatalf("copy body = %q", copiedBody)
	}
	if err := client.Delete(context.Background(), "a", files.OperationOptions{}); err != nil {
		t.Fatal(err)
	}
	if deleted != "a" {
		t.Fatalf("deleted = %q", deleted)
	}
	publicURL, err := client.URL(context.Background(), "a", files.URLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantURL := server.URL + "/storage/buckets/uploads/files/a/view?project=" + url.QueryEscape(testProject)
	if publicURL != wantURL {
		t.Fatalf("url = %q want %q", publicURL, wantURL)
	}
	if _, err := client.SignedUploadURL(context.Background(), "a", files.SignedUploadOptions{}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("signed upload error = %#v", err)
	}
}

func TestUnsupportedUploadOptionsRejectedByCore(t *testing.T) {
	adapter, err := New(Options{Bucket: testBucket, ProjectID: testProject})
	if err != nil {
		t.Fatal(err)
	}
	client := files.MustNew(files.Options{Adapter: adapter})
	if _, err := client.Upload(context.Background(), "a", files.StringBody("x"), files.UploadOptions{Metadata: map[string]string{"a": "b"}}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("metadata error = %#v", err)
	}
	if _, err := client.Upload(context.Background(), "a", files.StringBody("x"), files.UploadOptions{CacheControl: "public"}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("cacheControl error = %#v", err)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		code   files.ErrorCode
	}{
		{http.StatusNotFound, files.ErrNotFound},
		{http.StatusUnauthorized, files.ErrUnauthorized},
		{http.StatusForbidden, files.ErrUnauthorized},
		{http.StatusConflict, files.ErrConflict},
		{http.StatusPreconditionFailed, files.ErrConflict},
		{http.StatusInternalServerError, files.ErrProvider},
	}
	for _, tc := range cases {
		err := statusError(tc.status, []byte(`{"message":"boom"}`))
		if err.Code != tc.code {
			t.Fatalf("status %d got %s want %s", tc.status, err.Code, tc.code)
		}
	}
}

func TestResumableUpload(t *testing.T) {
	const fiveMiB = 5 * 1024 * 1024
	var ranges []string
	deleteCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/storage/buckets/uploads/files":
			if r.Header.Get("X-Appwrite-ID") != "doc" {
				t.Fatalf("missing upload id header: %#v", r.Header)
			}
			ranges = append(ranges, r.Header.Get("Content-Range"))
			fileID, data := readMultipartFile(t, r)
			if fileID != "doc" {
				t.Fatalf("fileID = %q", fileID)
			}
			writeJSON(t, w, http.StatusCreated, map[string]any{"$id": "doc", "mimeType": "application/octet-stream", "sizeOriginal": len(data)})
		case r.Method == http.MethodDelete && r.URL.Path == "/storage/buckets/uploads/files/doc":
			deleteCalled = true
			writeJSON(t, w, http.StatusNoContent, nil)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	adapter, err := New(Options{Bucket: testBucket, Endpoint: server.URL, ProjectID: testProject, Key: testKey})
	if err != nil {
		t.Fatal(err)
	}
	client := files.MustNew(files.Options{Adapter: adapter})
	control := files.NewUploadControl()
	data := bytes.Repeat([]byte("a"), fiveMiB+10)
	out, err := client.Upload(context.Background(), "doc", files.BytesBody(data), files.UploadOptions{
		ContentType: "application/octet-stream",
		Control:     control,
		Multipart:   &files.MultipartOptions{PartSize: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Key != "doc" || control.Status() != files.UploadControlCompleted {
		t.Fatalf("out=%#v status=%s", out, control.Status())
	}
	if strings.Join(ranges, ",") != "bytes 0-5242879/5242890,bytes 5242880-5242889/5242890" {
		t.Fatalf("ranges = %#v", ranges)
	}
	session, ok := control.Session()
	if !ok || session.Provider != "appwrite" || session.PartSize != fiveMiB || session.TempPath != "5242890" {
		t.Fatalf("session=%#v ok=%v", session, ok)
	}
	driver, err := adapter.ResumableUpload(context.Background(), "doc", files.ResumableUploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := driver.Adopt(context.Background(), files.ResumableSession{Provider: "s3", Key: "doc", Bucket: testBucket}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("adopt error = %#v", err)
	}
	if err := driver.Abort(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !deleteCalled {
		t.Fatal("abort did not delete partial file")
	}
}

func TestResumableAbortPropagatesDeleteError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete && r.URL.Path == "/storage/buckets/uploads/files/doc":
			writeJSON(t, w, http.StatusInternalServerError, map[string]any{"message": "delete failed"})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	adapter, err := New(Options{Bucket: testBucket, Endpoint: server.URL, ProjectID: testProject, Key: testKey})
	if err != nil {
		t.Fatal(err)
	}
	driver, err := adapter.ResumableUpload(context.Background(), "doc", files.ResumableUploadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := driver.Begin(context.Background(), files.ResumableUploadMeta{Key: "doc", Size: 1}); err != nil {
		t.Fatal(err)
	}
	if err := driver.Abort(context.Background()); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("abort error = %#v", err)
	}
}

func TestResumableRequiresAPIKey(t *testing.T) {
	adapter, err := New(Options{Bucket: testBucket, Endpoint: "https://example.test/v1", ProjectID: testProject})
	if err != nil {
		t.Fatal(err)
	}
	client := files.MustNew(files.Options{Adapter: adapter})
	if _, err := client.Upload(context.Background(), "doc", files.StringBody("data"), files.UploadOptions{Control: files.NewUploadControl()}); !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("api key error = %#v", err)
	}
}

func TestReadMultipartFileRejectsMalformed(t *testing.T) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.Close()
	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	fileID, data := readMultipartFile(t, req)
	if fileID != "" || len(data) != 0 {
		t.Fatalf("fileID=%q data=%q", fileID, data)
	}
}
