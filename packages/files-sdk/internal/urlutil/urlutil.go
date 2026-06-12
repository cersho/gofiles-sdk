package urlutil

import (
	"net/url"
	"strings"
)

func JoinPublicURL(base string, key string) string {
	trimmed := strings.TrimRight(base, "/")
	return trimmed + "/" + EscapeSegments(key)
}

func EscapeSegments(key string) string {
	parts := strings.Split(key, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
