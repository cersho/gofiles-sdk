package files

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

const DefaultResumablePartSize int64 = 5 * 1024 * 1024

type UploadControlStatus string

const (
	UploadControlIdle      UploadControlStatus = "idle"
	UploadControlUploading UploadControlStatus = "uploading"
	UploadControlPaused    UploadControlStatus = "paused"
	UploadControlCompleted UploadControlStatus = "completed"
	UploadControlAborted   UploadControlStatus = "aborted"
	UploadControlError     UploadControlStatus = "error"
)

type PartMeta struct {
	PartNumber int
	Size       int64
	ETag       string
}

type ResumableSession struct {
	Provider    string
	Key         string
	Bucket      string
	UploadID    string
	PartSize    int64
	ContentType string
	TempPath    string
	Parts       []PartMeta
}

type ResumableUploadMeta struct {
	Key          string
	Size         int64
	ContentType  string
	CacheControl string
	Metadata     map[string]string
	PartSize     int64
}

type ResumableUploadOptions struct {
	ContentType  string
	CacheControl string
	Metadata     map[string]string
	Multipart    *MultipartOptions
}

type ResumableProbe struct {
	NextOffset int64
	Parts      []PartMeta
}

type ResumablePart struct {
	PartNumber int
	Offset     int64
	Data       []byte
}

type ResumableDriver interface {
	Begin(context.Context, ResumableUploadMeta) (ResumableSession, error)
	Adopt(context.Context, ResumableSession) error
	Probe(context.Context) (ResumableProbe, error)
	UploadPart(context.Context, ResumablePart) (PartMeta, error)
	Complete(context.Context, []PartMeta) (UploadResult, error)
	Abort(context.Context) error
}

type ResumableAdapter interface {
	ResumableUpload(context.Context, string, ResumableUploadOptions) (ResumableDriver, error)
}

type UploadControl struct {
	mu      sync.Mutex
	status  UploadControlStatus
	loaded  int64
	total   int64
	session *ResumableSession
	paused  bool
	cancel  context.CancelFunc
	discard func(context.Context) error
}

func NewUploadControl() *UploadControl {
	return &UploadControl{status: UploadControlIdle}
}

func UploadControlFrom(session ResumableSession) *UploadControl {
	c := NewUploadControl()
	copied := cloneSession(session)
	c.session = &copied
	return c
}

func (c *UploadControl) Status() UploadControlStatus {
	if c == nil {
		return UploadControlIdle
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

func (c *UploadControl) Loaded() int64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.loaded
}

func (c *UploadControl) Total() int64 {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.total
}

func (c *UploadControl) Session() (ResumableSession, bool) {
	if c == nil {
		return ResumableSession{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == nil {
		return ResumableSession{}, false
	}
	return cloneSession(*c.session), true
}

func (c *UploadControl) Pause() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status == UploadControlCompleted || c.status == UploadControlAborted {
		return
	}
	c.paused = true
	if c.status == UploadControlUploading {
		c.status = UploadControlPaused
	}
}

func (c *UploadControl) Resume() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paused = false
	if c.status == UploadControlPaused {
		c.status = UploadControlUploading
	}
}

func (c *UploadControl) Abort(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.status == UploadControlCompleted || c.status == UploadControlAborted {
		c.mu.Unlock()
		return nil
	}
	c.status = UploadControlAborted
	c.paused = false
	cancel := c.cancel
	discard := c.discard
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if discard != nil {
		return discard(ctx)
	}
	return nil
}

func (c *UploadControl) begin(total int64, cancel context.CancelFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total = total
	c.cancel = cancel
	if c.status != UploadControlPaused {
		c.status = UploadControlUploading
	}
}

func (c *UploadControl) setSession(session ResumableSession, discard func(context.Context) error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	copied := cloneSession(session)
	c.session = &copied
	c.discard = discard
}

func (c *UploadControl) setProgress(loaded int64, parts []PartMeta) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loaded = loaded
	if c.session != nil {
		c.session.Parts = cloneParts(parts)
	}
}

func (c *UploadControl) complete() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.status = UploadControlCompleted
	c.loaded = c.total
	c.paused = false
	c.cancel = nil
	c.discard = nil
}

func (c *UploadControl) fail() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.status != UploadControlAborted {
		c.status = UploadControlError
	}
	c.cancel = nil
}

func (c *UploadControl) wait(ctx context.Context) error {
	for {
		c.mu.Lock()
		status := c.status
		paused := c.paused
		c.mu.Unlock()
		if status == UploadControlAborted {
			return context.Canceled
		}
		if !paused {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (c *Client) runResumableUpload(ctx context.Context, key string, path string, body Body, opts UploadOptions, action actionContext) (UploadResult, error) {
	control := opts.Control
	if control == nil {
		return UploadResult{}, NewError(ErrProvider, "upload control is required", nil)
	}
	size, ok := body.Size()
	if !ok {
		return UploadResult{}, NewError(ErrProvider, "resumable upload requires a known-size body", nil)
	}
	if !body.Replayable() {
		return UploadResult{}, NewError(ErrProvider, "resumable upload requires a replayable body", nil)
	}
	adapter, ok := c.adapter.(ResumableAdapter)
	if !ok || !c.adapter.Capabilities().Resumable {
		return UploadResult{}, NewError(ErrProvider, c.adapter.Name()+": resumable uploads are not supported by this adapter", nil)
	}
	contentType := effectiveContentType(body, opts.ContentType)
	partSize := DefaultResumablePartSize
	if opts.Multipart != nil && opts.Multipart.PartSize > 0 {
		partSize = opts.Multipart.PartSize
	}
	if partSize <= 0 {
		partSize = DefaultResumablePartSize
	}

	result, err := c.run(ctx, opts.OperationOptions, false, action, func(runCtx context.Context) (any, error) {
		uploadCtx, cancel := context.WithCancel(runCtx)
		control.begin(size, cancel)
		defer cancel()
		driver, err := adapter.ResumableUpload(uploadCtx, path, ResumableUploadOptions{
			ContentType:  contentType,
			CacheControl: opts.CacheControl,
			Metadata:     cloneStringMap(opts.Metadata),
			Multipart:    opts.Multipart,
		})
		if err != nil {
			return UploadResult{}, err
		}
		var session ResumableSession
		if existing, ok := control.Session(); ok {
			if existing.PartSize > 0 {
				partSize = existing.PartSize
			}
			if err := driver.Adopt(uploadCtx, existing); err != nil {
				return UploadResult{}, err
			}
			session = existing
		} else {
			session, err = driver.Begin(uploadCtx, ResumableUploadMeta{
				Key:          path,
				Size:         size,
				ContentType:  contentType,
				CacheControl: opts.CacheControl,
				Metadata:     cloneStringMap(opts.Metadata),
				PartSize:     partSize,
			})
			if err != nil {
				return UploadResult{}, err
			}
			if session.PartSize > 0 {
				partSize = session.PartSize
			}
		}
		control.setSession(session, driver.Abort)
		out, err := uploadResumableParts(uploadCtx, control, driver, body, size, partSize, opts.OnProgress)
		if err != nil {
			control.fail()
			return UploadResult{}, err
		}
		control.complete()
		return out, nil
	})
	if err != nil {
		return UploadResult{}, err
	}
	out := result.(UploadResult)
	out.Key = path
	return c.uploadResult(out), nil
}

func uploadResumableParts(ctx context.Context, control *UploadControl, driver ResumableDriver, body Body, total int64, partSize int64, onProgress func(UploadProgress)) (UploadResult, error) {
	probe, err := driver.Probe(ctx)
	if err != nil {
		return UploadResult{}, err
	}
	parts := cloneParts(probe.Parts)
	offset := probe.NextOffset
	if offset < 0 || offset > total {
		return UploadResult{}, NewError(ErrProvider, "resumable upload probe returned an invalid offset", nil)
	}
	control.setProgress(offset, parts)
	emitHook(onProgress, UploadProgress{Loaded: offset, Total: total, Known: true})

	reader, err := body.Open(ctx)
	if err != nil {
		return UploadResult{}, err
	}
	defer reader.Close()
	if offset > 0 {
		if _, err := io.CopyN(io.Discard, reader, offset); err != nil {
			return UploadResult{}, err
		}
	}

	buffer := make([]byte, int(partSize))
	for offset < total {
		if err := control.wait(ctx); err != nil {
			return UploadResult{}, err
		}
		n, readErr := io.ReadFull(reader, buffer)
		if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
			return UploadResult{}, readErr
		}
		if n == 0 {
			break
		}
		partNumber := int(offset/partSize) + 1
		data := append([]byte(nil), buffer[:n]...)
		part, err := driver.UploadPart(ctx, ResumablePart{
			PartNumber: partNumber,
			Offset:     offset,
			Data:       data,
		})
		if err != nil {
			return UploadResult{}, err
		}
		parts = upsertPart(parts, part)
		offset += int64(n)
		control.setProgress(offset, parts)
		emitHook(onProgress, UploadProgress{Loaded: offset, Total: total, Known: true})
		if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
			break
		}
	}
	if offset != total {
		return UploadResult{}, NewError(ErrProvider, "resumable upload ended before all bytes were read", nil)
	}
	return driver.Complete(ctx, parts)
}

func upsertPart(parts []PartMeta, part PartMeta) []PartMeta {
	for i := range parts {
		if parts[i].PartNumber == part.PartNumber {
			parts[i] = part
			return parts
		}
	}
	return append(parts, part)
}

func cloneParts(in []PartMeta) []PartMeta {
	return append([]PartMeta(nil), in...)
}

func cloneSession(in ResumableSession) ResumableSession {
	out := in
	out.Parts = cloneParts(in.Parts)
	return out
}
