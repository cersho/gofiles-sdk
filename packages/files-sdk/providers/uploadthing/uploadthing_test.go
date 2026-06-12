package uploadthing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
)

func testToken() string {
	return base64.StdEncoding.EncodeToString([]byte(`{"apiKey":"sk_test","appId":"myapp","regions":["sea1"]}`))
}

func TestNewValidatesToken(t *testing.T) {
	_, err := New(Options{Token: "not-base64"})
	if !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("error = %#v", err)
	}
}

func TestURLReturnsPublicCustomIDURL(t *testing.T) {
	adapter, err := New(Options{Token: testToken()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.URL(context.Background(), "avatars/me.png", files.URLOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := "https://myapp.ufs.sh/f/avatars%2Fme.png"
	if got != want {
		t.Fatalf("url = %q, want %q", got, want)
	}
	_, err = adapter.URL(context.Background(), "avatars/me.png", files.URLOptions{
		ResponseContentDisposition: "attachment",
	})
	if !files.IsCode(err, files.ErrProvider) {
		t.Fatalf("responseContentDisposition error = %#v", err)
	}
}

func TestSignedUploadURLIncludesCustomIDAndSignature(t *testing.T) {
	adapter, err := New(Options{Token: testToken(), Slug: "mediaUploader"})
	if err != nil {
		t.Fatal(err)
	}
	maxSize := int64(10_000_000)
	out, err := adapter.SignedUploadURL(context.Background(), "uploads/x.png", files.SignedUploadOptions{
		ContentType: "image/png",
		ExpiresIn:   time.Minute,
		MaxSize:     &maxSize,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Method != http.MethodPut {
		t.Fatalf("method = %q", out.Method)
	}
	u, err := url.Parse(out.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "sea1.ingest.uploadthing.com" {
		t.Fatalf("host = %q", u.Host)
	}
	q := u.Query()
	assertQuery := func(key string, want string) {
		t.Helper()
		if got := q.Get(key); got != want {
			t.Fatalf("%s = %q, want %q", key, got, want)
		}
	}
	assertQuery("x-ut-identifier", "myapp")
	assertQuery("x-ut-file-name", "x.png")
	assertQuery("x-ut-file-size", "10000000")
	assertQuery("x-ut-slug", "mediaUploader")
	assertQuery("x-ut-file-type", "image/png")
	assertQuery("x-ut-custom-id", "uploads/x.png")
	assertQuery("x-ut-acl", "public-read")
	signature := q.Get("signature")
	if !strings.HasPrefix(signature, "hmac-sha256=") || len(strings.TrimPrefix(signature, "hmac-sha256=")) != 64 {
		t.Fatalf("signature = %q", signature)
	}

	q.Del("signature")
	u.RawQuery = q.Encode()
	wantSignature := "hmac-sha256=" + hmacSHA256Hex(u.String(), "sk_test")
	if signature != wantSignature {
		t.Fatalf("signature = %q, want %q", signature, wantSignature)
	}
}

func TestRegionOverrideIsUsedForSignedUploadURL(t *testing.T) {
	adapter, err := New(Options{Token: testToken(), Region: "fra1"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := adapter.SignedUploadURL(context.Background(), "a.txt", files.SignedUploadOptions{ExpiresIn: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(out.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Host != "fra1.ingest.uploadthing.com" {
		t.Fatalf("host = %q", u.Host)
	}
}

func TestListAndDeleteUseUploadThingAPI(t *testing.T) {
	var listBody map[string]any
	var deleteBody map[string]any
	var sawHeaders bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-uploadthing-api-key") != "sk_test" ||
			r.Header.Get("x-uploadthing-be-adapter") != "server-sdk" ||
			r.Header.Get("x-uploadthing-version") == "" {
			t.Fatalf("missing UploadThing headers: %#v", r.Header)
		}
		sawHeaders = true
		switch r.URL.Path {
		case "/v6/listFiles":
			if err := json.NewDecoder(r.Body).Decode(&listBody); err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"hasMore":true,"files":[{"customId":"a/1.txt","key":"ut-key","size":3,"uploadedAt":1700000000000},{"customId":null,"key":"raw.txt","size":5,"uploadedAt":1700000001000}]}`))
		case "/v6/deleteFiles":
			if err := json.NewDecoder(r.Body).Decode(&deleteBody); err != nil {
				t.Fatal(err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	adapter, err := New(Options{Token: testToken(), APIURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	list, err := adapter.List(context.Background(), files.ListOptions{Prefix: "a/", Cursor: "5", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Items) != 1 || list.Items[0].Key != "a/1.txt" {
		t.Fatalf("items = %#v", list.Items)
	}
	if list.Cursor != "7" {
		t.Fatalf("cursor = %q", list.Cursor)
	}
	if listBody["offset"].(float64) != 5 || listBody["limit"].(float64) != 10 {
		t.Fatalf("list body = %#v", listBody)
	}

	out, err := adapter.DeleteMany(context.Background(), []string{"a/1.txt", "b/2.txt"}, files.DeleteManyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Deleted) != 2 {
		t.Fatalf("deleted = %#v", out.Deleted)
	}
	rawIDs, ok := deleteBody["customIds"].([]any)
	if !ok || len(rawIDs) != 2 || rawIDs[0] != "a/1.txt" || rawIDs[1] != "b/2.txt" {
		t.Fatalf("delete body = %#v", deleteBody)
	}
	if !sawHeaders {
		t.Fatal("expected UploadThing headers")
	}
}
