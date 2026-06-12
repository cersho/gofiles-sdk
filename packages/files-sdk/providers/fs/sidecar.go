package fs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	files "github.com/cersho/gofiles-sdk/packages/files-sdk"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/typeutil"
)

const sidecarSuffix = ".meta.json"
const resumableSuffix = ".fls-part"

type sidecar struct {
	ContentType  string            `json:"contentType"`
	CacheControl string            `json:"cacheControl,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	ETag         string            `json:"etag"`
	LastModified time.Time         `json:"lastModified"`
}

func sidecarPath(target string) string { return target + sidecarSuffix }

func readSidecar(target string) (sidecar, error) {
	raw, err := os.ReadFile(sidecarPath(target))
	if err != nil {
		return sidecar{}, mapFSError(err)
	}
	var meta sidecar
	if err := json.Unmarshal(raw, &meta); err != nil {
		return sidecar{}, files.NewError(files.ErrProvider, "fs: invalid metadata sidecar", err)
	}
	if meta.ContentType == "" {
		meta.ContentType = typeutil.GenericContentType
	}
	return meta, nil
}

func writeSidecar(target string, meta sidecar) error {
	raw, err := json.Marshal(meta)
	if err != nil {
		return files.NewError(files.ErrProvider, err.Error(), err)
	}
	return mapFSError(os.WriteFile(sidecarPath(target), raw, 0o644))
}

func isReserved(p string) bool {
	base := strings.ToLower(filepath.Base(p))
	return strings.HasSuffix(base, sidecarSuffix) || strings.HasSuffix(base, resumableSuffix)
}
