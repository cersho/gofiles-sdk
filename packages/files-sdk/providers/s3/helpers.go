package s3

import (
	"net/http"
	"strings"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/maputil"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/rangeutil"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/urlutil"
)

func stripETag(etag string) string {
	return strings.Trim(etag, "\"")
}

func rangeHeader(r files.ByteRange) string {
	return rangeutil.Header(r.Start, r.End)
}

func headerMap(header http.Header) map[string]string {
	if len(header) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, values := range header {
		if len(values) > 0 {
			out[key] = values[0]
		}
	}
	return out
}

func joinPublicURL(base string, key string) string {
	return urlutil.JoinPublicURL(base, key)
}

func cloneStringMap(in map[string]string) map[string]string {
	return maputil.CloneStringMap(in)
}
