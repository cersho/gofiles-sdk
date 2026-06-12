package files

import (
	"context"
	"sync"
)

const defaultBulkConcurrency = 8

func bulkConcurrency(n int) int {
	if n <= 0 {
		return defaultBulkConcurrency
	}
	return n
}

func (c *Client) UploadMany(ctx context.Context, items []UploadManyItem, opts UploadManyOptions) (UploadManyResult, error) {
	if err := c.assertWritable(ActionUpload); err != nil {
		return UploadManyResult{}, err
	}
	result, err := c.action(ctx, actionContext{Type: ActionUpload, Keys: uploadManyKeys(items)}, func(ctx context.Context) (any, error) {
		type indexed struct {
			index int
			item  UploadManyItem
		}
		results := make([]UploadResult, len(items))
		var errors []BulkError
		var mu sync.Mutex
		work := make(chan indexed)
		worker := func() {
			for job := range work {
				itemOpts := UploadOptions{
					ContentType:  job.item.ContentType,
					CacheControl: job.item.CacheControl,
					Metadata:     cloneStringMap(job.item.Metadata),
					Multipart:    job.item.Multipart,
				}
				if opts.OnProgress != nil {
					itemOpts.OnProgress = func(p UploadProgress) {
						opts.OnProgress(job.item.Key, p)
					}
				}
				out, err := c.runUpload(ctx, job.item.Key, job.item.Body, itemOpts, actionContext{Type: ActionUpload, Key: job.item.Key})
				mu.Lock()
				if err != nil {
					errors = append(errors, BulkError{Key: job.item.Key, Error: WrapError(err, ErrProvider)})
				} else {
					results[job.index] = out
				}
				mu.Unlock()
			}
		}
		concurrency := bulkConcurrency(opts.Concurrency)
		if opts.StopOnError {
			concurrency = 1
		}
		var wg sync.WaitGroup
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); worker() }()
		}
		for i, item := range items {
			if opts.StopOnError {
				mu.Lock()
				hasError := len(errors) > 0
				mu.Unlock()
				if hasError {
					break
				}
			}
			work <- indexed{index: i, item: item}
		}
		close(work)
		wg.Wait()
		uploaded := make([]UploadResult, 0, len(items))
		for _, result := range results {
			if result.Key != "" {
				uploaded = append(uploaded, result)
			}
		}
		return UploadManyResult{Uploaded: uploaded, Errors: errors}, nil
	})
	if err != nil {
		return UploadManyResult{}, err
	}
	return result.(UploadManyResult), nil
}

func uploadManyKeys(items []UploadManyItem) []string {
	keys := make([]string, 0, len(items))
	for _, item := range items {
		keys = append(keys, item.Key)
	}
	return keys
}

func (c *Client) DownloadMany(ctx context.Context, keys []string, opts DownloadManyOptions) (DownloadManyResult, error) {
	result, err := c.bulkMap(ctx, actionContext{Type: ActionDownload, Keys: keys}, keys, opts.BulkOptions, func(key string) (StoredFile, error) {
		return c.Download(ctx, key, DownloadOptions{Range: opts.Range})
	})
	if err != nil {
		return DownloadManyResult{}, err
	}
	return DownloadManyResult{Downloaded: result.files, Errors: result.errors}, nil
}

func (c *Client) HeadMany(ctx context.Context, keys []string, opts BulkOptions) (HeadManyResult, error) {
	result, err := c.bulkMap(ctx, actionContext{Type: ActionHead, Keys: keys}, keys, opts, func(key string) (StoredFile, error) {
		return c.Head(ctx, key, OperationOptions{})
	})
	if err != nil {
		return HeadManyResult{}, err
	}
	return HeadManyResult{Files: result.files, Errors: result.errors}, nil
}

func (c *Client) ExistsMany(ctx context.Context, keys []string, opts BulkOptions) (ExistsManyResult, error) {
	result, err := c.action(ctx, actionContext{Type: ActionExists, Keys: keys}, func(context.Context) (any, error) {
		type indexed struct {
			index int
			key   string
		}
		type existsResult struct {
			key    string
			exists bool
		}
		values := make([]existsResult, len(keys))
		okSlots := make([]bool, len(keys))
		errors := []BulkError{}
		work := make(chan indexed)
		var mu sync.Mutex
		worker := func() {
			for job := range work {
				ok, err := c.Exists(ctx, job.key, OperationOptions{})
				mu.Lock()
				if err != nil {
					errors = append(errors, BulkError{Key: job.key, Error: WrapError(err, ErrProvider)})
				} else {
					values[job.index] = existsResult{key: job.key, exists: ok}
					okSlots[job.index] = true
				}
				mu.Unlock()
			}
		}
		concurrency := bulkConcurrency(opts.Concurrency)
		if opts.StopOnError {
			concurrency = 1
		}
		var wg sync.WaitGroup
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); worker() }()
		}
		for i, key := range keys {
			if opts.StopOnError {
				mu.Lock()
				hasError := len(errors) > 0
				mu.Unlock()
				if hasError {
					break
				}
			}
			work <- indexed{index: i, key: key}
		}
		close(work)
		wg.Wait()
		out := ExistsManyResult{Errors: errors}
		for i, value := range values {
			if !okSlots[i] {
				continue
			}
			if value.exists {
				out.Existing = append(out.Existing, value.key)
			} else {
				out.Missing = append(out.Missing, value.key)
			}
		}
		return out, nil
	})
	if err != nil {
		return ExistsManyResult{}, err
	}
	return result.(ExistsManyResult), nil
}

func (c *Client) DeleteMany(ctx context.Context, keys []string, opts DeleteManyOptions) (DeleteManyResult, error) {
	if err := c.assertWritable(ActionDelete); err != nil {
		return DeleteManyResult{}, err
	}
	result, err := c.action(ctx, actionContext{Type: ActionDelete, Keys: keys}, func(ctx context.Context) (any, error) {
		if len(c.middleware) == 0 {
			if native, ok := c.adapter.(DeleteManyAdapter); ok {
				paths := make([]string, 0, len(keys))
				keyByPath := map[string]string{}
				validationErrors := []DeleteManyError{}
				for _, key := range keys {
					path, err := c.path(key, "key")
					if err != nil {
						validationErrors = append(validationErrors, DeleteManyError{Key: key, Error: WrapError(err, ErrProvider)})
						if opts.StopOnError {
							return DeleteManyResult{Errors: validationErrors}, nil
						}
						continue
					}
					paths = append(paths, path)
					keyByPath[path] = key
				}
				if len(paths) == 0 {
					return DeleteManyResult{Errors: validationErrors}, nil
				}
				out, err := native.DeleteMany(ctx, paths, opts)
				if err != nil {
					mapped := WrapError(err, ErrProvider)
					errors := make([]DeleteManyError, 0, len(validationErrors)+len(paths))
					errors = append(errors, validationErrors...)
					for _, path := range paths {
						errors = append(errors, DeleteManyError{Key: keyByPath[path], Error: mapped})
					}
					return DeleteManyResult{Errors: errors}, nil
				}
				for i := range out.Deleted {
					if key := keyByPath[out.Deleted[i]]; key != "" {
						out.Deleted[i] = key
					}
				}
				for i := range out.Errors {
					if key := keyByPath[out.Errors[i].Key]; key != "" {
						out.Errors[i].Key = key
					}
				}
				out.Errors = append(validationErrors, out.Errors...)
				return out, nil
			}
		}
		deleted := []string{}
		errors := []DeleteManyError{}
		concurrency := bulkConcurrency(opts.Concurrency)
		if opts.StopOnError {
			concurrency = 1
		}
		work := make(chan string)
		var mu sync.Mutex
		var wg sync.WaitGroup
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for key := range work {
					err := c.Delete(ctx, key, OperationOptions{})
					mu.Lock()
					if err != nil {
						errors = append(errors, DeleteManyError{Key: key, Error: WrapError(err, ErrProvider)})
					} else {
						deleted = append(deleted, key)
					}
					mu.Unlock()
				}
			}()
		}
		for _, key := range keys {
			if opts.StopOnError {
				mu.Lock()
				hasError := len(errors) > 0
				mu.Unlock()
				if hasError {
					break
				}
			}
			work <- key
		}
		close(work)
		wg.Wait()
		return DeleteManyResult{Deleted: deleted, Errors: errors}, nil
	})
	if err != nil {
		return DeleteManyResult{}, err
	}
	return result.(DeleteManyResult), nil
}

type bulkFileResult struct {
	files  []StoredFile
	errors []BulkError
}

func (c *Client) bulkMap(ctx context.Context, action actionContext, keys []string, opts BulkOptions, fn func(string) (StoredFile, error)) (bulkFileResult, error) {
	result, err := c.action(ctx, action, func(context.Context) (any, error) {
		type indexed struct {
			index int
			key   string
		}
		values := make([]StoredFile, len(keys))
		ok := make([]bool, len(keys))
		errors := []BulkError{}
		work := make(chan indexed)
		var mu sync.Mutex
		worker := func() {
			for job := range work {
				value, err := fn(job.key)
				mu.Lock()
				if err != nil {
					errors = append(errors, BulkError{Key: job.key, Error: WrapError(err, ErrProvider)})
				} else {
					values[job.index] = value
					ok[job.index] = true
				}
				mu.Unlock()
			}
		}
		concurrency := bulkConcurrency(opts.Concurrency)
		if opts.StopOnError {
			concurrency = 1
		}
		var wg sync.WaitGroup
		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); worker() }()
		}
		for i, key := range keys {
			if opts.StopOnError {
				mu.Lock()
				hasError := len(errors) > 0
				mu.Unlock()
				if hasError {
					break
				}
			}
			work <- indexed{index: i, key: key}
		}
		close(work)
		wg.Wait()
		compact := make([]StoredFile, 0, len(keys))
		for i, value := range values {
			if ok[i] {
				compact = append(compact, value)
			}
		}
		return bulkFileResult{files: compact, errors: errors}, nil
	})
	if err != nil {
		return bulkFileResult{}, err
	}
	return result.(bulkFileResult), nil
}
