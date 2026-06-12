package files

import (
	"context"
	"time"
)

type Options struct {
	Adapter    Adapter
	Prefix     string
	ReadOnly   bool
	Timeout    time.Duration
	Retries    *RetryOptions
	Hooks      Hooks
	Middleware []Middleware
}

type Client struct {
	adapter    Adapter
	defaults   OperationOptions
	hooks      Hooks
	readOnly   bool
	prefix     string
	middleware []Middleware
}

func New(opts Options) (*Client, error) {
	if opts.Adapter == nil {
		return nil, NewError(ErrProvider, "adapter is required", nil)
	}
	prefix, err := normalizePrefix(opts.Prefix)
	if err != nil {
		return nil, err
	}
	return &Client{
		adapter:    opts.Adapter,
		defaults:   OperationOptions{Timeout: opts.Timeout, Retries: opts.Retries},
		hooks:      opts.Hooks,
		readOnly:   opts.ReadOnly,
		prefix:     prefix,
		middleware: append([]Middleware(nil), opts.Middleware...),
	}, nil
}

func MustNew(opts Options) *Client {
	c, err := New(opts)
	if err != nil {
		panic(err)
	}
	return c
}

func (c *Client) Adapter() Adapter {
	return c.adapter
}

func (c *Client) Raw() any {
	return c.adapter.Raw()
}

func (c *Client) Prefix() string {
	return c.prefix
}

func (c *Client) Capabilities() AdapterCapabilities {
	return c.adapter.Capabilities()
}

func (c *Client) ReadOnly() *Client {
	clone := *c
	clone.readOnly = true
	return &clone
}

func (c *Client) File(key string) FileHandle {
	return FileHandle{client: c, key: key}
}

func (c *Client) dispatch(ctx context.Context, op Operation, base Handler) (any, error) {
	if len(c.middleware) == 0 {
		return base(ctx, op)
	}
	chain := base
	for i := len(c.middleware) - 1; i >= 0; i-- {
		mw := c.middleware[i]
		next := chain
		chain = func(ctx context.Context, op Operation) (any, error) {
			return mw(ctx, op, next)
		}
	}
	return chain(ctx, op)
}

func (c *Client) action(ctx context.Context, action actionContext, fn func(context.Context) (any, error)) (any, error) {
	if c.hooks.OnAction == nil && c.hooks.OnError == nil {
		return fn(ctx)
	}
	start := time.Now()
	result, err := fn(ctx)
	duration := time.Since(start)
	if err == nil {
		emitHook(c.hooks.OnAction, ActionEvent{
			Type:     action.Type,
			Key:      action.Key,
			Keys:     action.Keys,
			From:     action.From,
			To:       action.To,
			Duration: duration,
			Status:   "success",
			Result:   result,
		})
		return result, nil
	}
	wrapped := WrapError(err, ErrProvider)
	emitHook(c.hooks.OnError, ErrorEvent{
		Type:     action.Type,
		Key:      action.Key,
		Keys:     action.Keys,
		From:     action.From,
		To:       action.To,
		Duration: duration,
		Error:    wrapped,
	})
	emitHook(c.hooks.OnAction, ActionEvent{
		Type:     action.Type,
		Key:      action.Key,
		Keys:     action.Keys,
		From:     action.From,
		To:       action.To,
		Duration: duration,
		Status:   "error",
		Error:    wrapped,
	})
	return nil, wrapped
}

func (c *Client) run(ctx context.Context, opts OperationOptions, retryable bool, action actionContext, fn func(context.Context) (any, error)) (any, error) {
	effective := mergeOperation(c.defaults, opts)
	maxRetries := 0
	if retryable && effective.Retries != nil && effective.Retries.Max > 0 {
		maxRetries = effective.Retries.Max
	}
	for attempt := 0; ; attempt++ {
		attemptCtx, cancel := withAttemptTimeout(ctx, effective.Timeout)
		result, err := fn(attemptCtx)
		cancel()
		if err == nil {
			return result, nil
		}
		wrapped := WrapError(err, ErrProvider)
		if attemptCtx.Err() != nil {
			wrapped = WrapError(attemptCtx.Err(), ErrProvider)
			if effective.Timeout > 0 && attemptCtx.Err() == context.DeadlineExceeded {
				wrapped = TimeoutError(effective.Timeout.String())
			}
		}
		if !canRetry(wrapped, attempt, maxRetries) {
			return nil, wrapped
		}
		delay := defaultBackoff(attempt + 1)
		if effective.Retries != nil && effective.Retries.Backoff != nil {
			delay = effective.Retries.Backoff(RetryBackoffContext{
				Attempt: attempt + 1,
				Error:   wrapped,
			})
			if delay < 0 {
				delay = 0
			}
		}
		emitHook(c.hooks.OnRetry, RetryEvent{
			Type:       action.Type,
			Key:        action.Key,
			From:       action.From,
			To:         action.To,
			Attempt:    attempt + 1,
			MaxRetries: maxRetries,
			Delay:      delay,
			Error:      wrapped,
		})
		if err := sleepContext(ctx, delay); err != nil {
			return nil, WrapError(err, ErrProvider)
		}
	}
}

func (c *Client) path(key string, label string) (string, error) {
	if err := assertValidKey(key, label); err != nil {
		return "", err
	}
	if c.prefix == "" {
		return key, nil
	}
	normalized := stringsTrimLeftSlash(key)
	if err := assertNoRelativeSegments(normalized, label); err != nil {
		return "", err
	}
	return c.prefix + "/" + normalized, nil
}

func stringsTrimLeftSlash(value string) string {
	for stringsHasPrefix(value, "/") {
		value = value[1:]
	}
	return value
}

func stringsHasPrefix(s string, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func (c *Client) stripPrefix(key string) string {
	if c.prefix == "" {
		return key
	}
	scoped := c.prefix + "/"
	if stringsHasPrefix(key, scoped) {
		return key[len(scoped):]
	}
	return key
}

func (c *Client) storedFile(file StoredFile) StoredFile {
	if c.prefix == "" {
		return file
	}
	file.Key = c.stripPrefix(file.Key)
	file.Name = c.stripPrefix(file.Name)
	return file
}

func (c *Client) uploadResult(result UploadResult) UploadResult {
	if c.prefix == "" {
		return result
	}
	result.Key = c.stripPrefix(result.Key)
	return result
}

func (c *Client) assertWritable(action ActionType) error {
	if !c.readOnly {
		return nil
	}
	return NewError(ErrReadOnly, "cannot call "+string(action)+" on a read-only Files instance", nil)
}

func (c *Client) assertUploadOptionsSupported(opts UploadOptions) error {
	caps := c.adapter.Capabilities()
	if len(opts.Metadata) > 0 && !caps.Metadata {
		return NewError(ErrProvider, c.adapter.Name()+": metadata is not supported by this adapter", nil)
	}
	if opts.CacheControl != "" && !caps.CacheControl {
		return NewError(ErrProvider, c.adapter.Name()+": cacheControl is not supported by this adapter", nil)
	}
	return nil
}

func (c *Client) assertRangeSupported(r *ByteRange) error {
	if err := validateRange(r); err != nil {
		return err
	}
	if r != nil && !c.adapter.Capabilities().RangeRead {
		return NewError(ErrProvider, c.adapter.Name()+": range downloads are not supported by this adapter", nil)
	}
	return nil
}

func (c *Client) assertDelimiterSupported(opts ListOptions) error {
	if opts.Delimiter == "" {
		return nil
	}
	if !c.adapter.Capabilities().Delimiter {
		return NewError(ErrProvider, c.adapter.Name()+": directory-style listing is not supported by this adapter", nil)
	}
	return nil
}
