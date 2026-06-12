package files

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/maputil"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/rangeutil"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/typeutil"
	"github.com/cersho/gofiles-sdk/packages/files-sdk/internal/urlutil"
)

func cloneStringMap(in map[string]string) map[string]string {
	return maputil.CloneStringMap(in)
}

func normalizePrefix(prefix string) (string, error) {
	if prefix == "" {
		return "", nil
	}
	normalized := strings.Trim(prefix, "/")
	if err := assertValidKey(normalized, "prefix"); err != nil {
		return "", err
	}
	if err := assertNoRelativeSegments(normalized, "prefix"); err != nil {
		return "", err
	}
	return normalized, nil
}

func assertValidKey(key string, label string) error {
	if key == "" {
		return NewError(ErrProvider, label+" must be a non-empty string", nil)
	}
	if strings.ContainsRune(key, '\x00') {
		return NewError(ErrProvider, label+" must not contain null bytes", nil)
	}
	return nil
}

func assertNoRelativeSegments(key string, label string) error {
	for _, segment := range strings.Split(key, "/") {
		if segment == "." || segment == ".." {
			return NewError(ErrProvider, label+" must not contain . or .. path segments", nil)
		}
	}
	return nil
}

func joinPublicURL(base string, key string) string {
	return urlutil.JoinPublicURL(base, key)
}

func httpRangeHeader(r ByteRange) string {
	return rangeutil.Header(r.Start, r.End)
}

func validateRange(r *ByteRange) error {
	if r == nil {
		return nil
	}
	if r.Start < 0 {
		return NewError(ErrProvider, "range.start must be a non-negative integer", nil)
	}
	if r.End != nil && *r.End < r.Start {
		return NewError(ErrProvider, "range.end must be greater than or equal to range.start", nil)
	}
	return nil
}

func rangedSize(fullSize int64, r ByteRange) int64 {
	return rangeutil.Size(fullSize, r.Start, r.End)
}

func effectiveContentType(body Body, hint string) string {
	return typeutil.EffectiveContentType(hint, body.ContentType())
}

func withAttemptTimeout(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return parent, func() {}
	}
	return context.WithTimeout(parent, timeout)
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func defaultBackoff(attempt int) time.Duration {
	backoff := 100 * time.Millisecond * time.Duration(1<<max(0, attempt-1))
	if backoff > 30*time.Second {
		return 30 * time.Second
	}
	return backoff
}

func canRetry(err *Error, attempt int, maxAttempts int) bool {
	return attempt < maxAttempts &&
		err != nil &&
		err.Code == ErrProvider &&
		!err.Aborted &&
		!err.Permanent
}

func firstNonZeroDuration(values ...time.Duration) time.Duration {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func mergeOperation(defaults OperationOptions, perCall OperationOptions) OperationOptions {
	out := defaults
	if perCall.Timeout != 0 {
		out.Timeout = perCall.Timeout
	}
	if perCall.Retries != nil {
		out.Retries = perCall.Retries
	}
	return out
}

func regexpFromGlob(pattern string, caseInsensitive bool) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	flags := ""
	if caseInsensitive {
		flags = "(?i)"
	}
	re, err := regexp.Compile(flags + b.String())
	if err != nil {
		return nil, NewError(ErrProvider, fmt.Sprintf("search pattern is not a valid glob: %s", pattern), err)
	}
	return re, nil
}

func globPrefix(pattern string) string {
	stop := len(pattern)
	for _, token := range []string{"*", "?", "["} {
		if idx := strings.Index(pattern, token); idx >= 0 && idx < stop {
			stop = idx
		}
	}
	head := pattern[:stop]
	if idx := strings.LastIndex(head, "/"); idx >= 0 {
		return head[:idx+1]
	}
	if stop == len(pattern) {
		return pattern
	}
	return ""
}

func buildSearchMatcher(pattern string, opts SearchOptions) (func(string) bool, string, error) {
	match := opts.Match
	if match == "" {
		match = SearchGlob
	}
	switch match {
	case SearchSubstring:
		needle := pattern
		if opts.CaseInsensitive {
			needle = strings.ToLower(needle)
		}
		return func(key string) bool {
			if opts.CaseInsensitive {
				key = strings.ToLower(key)
			}
			return strings.Contains(key, needle)
		}, opts.Prefix, nil
	case SearchExact:
		needle := pattern
		if opts.CaseInsensitive {
			needle = strings.ToLower(needle)
		}
		return func(key string) bool {
			if opts.CaseInsensitive {
				key = strings.ToLower(key)
			}
			return key == needle
		}, opts.Prefix, nil
	case SearchRegex:
		flags := ""
		if opts.CaseInsensitive {
			flags = "(?i)"
		}
		re, err := regexp.Compile(flags + pattern)
		if err != nil {
			return nil, "", NewError(ErrProvider, "search pattern is not a valid regular expression: "+pattern, err)
		}
		return re.MatchString, opts.Prefix, nil
	case SearchGlob, "":
		re, err := regexpFromGlob(pattern, opts.CaseInsensitive)
		if err != nil {
			return nil, "", err
		}
		prefix := opts.Prefix
		if prefix == "" && !opts.CaseInsensitive {
			prefix = globPrefix(pattern)
		}
		return re.MatchString, prefix, nil
	default:
		return nil, "", NewError(ErrProvider, "unsupported search match mode: "+string(match), nil)
	}
}
