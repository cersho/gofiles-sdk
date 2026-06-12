package files

import (
	"context"
	"errors"
	"sync"
)

type TransferProgress struct {
	Done   int
	Total  int
	Key    string
	Status string
}

type TransferOptions struct {
	BulkOptions
	Prefix       string
	TransformKey func(string) string
	Overwrite    *bool
	Limit        int32
	OnProgress   func(TransferProgress)
}

type TransferResult struct {
	Transferred []string
	Skipped     []string
	Errors      []BulkError
}

func Transfer(ctx context.Context, source *Client, dest *Client, opts TransferOptions) (TransferResult, error) {
	if source == nil || dest == nil {
		return TransferResult{}, NewError(ErrProvider, "transfer requires source and destination clients", nil)
	}
	transform := opts.TransformKey
	if transform == nil {
		transform = func(key string) string { return key }
	}
	overwrite := true
	if opts.Overwrite != nil {
		overwrite = *opts.Overwrite
	}
	var keys []string
	err := source.ListAll(ctx, ListOptions{Prefix: opts.Prefix, Limit: opts.Limit}, func(file StoredFile) error {
		keys = append(keys, file.Key)
		return nil
	})
	if err != nil {
		return TransferResult{}, err
	}
	total := len(keys)
	concurrency := bulkConcurrency(opts.Concurrency)
	if opts.StopOnError {
		concurrency = 1
	}
	type indexed struct {
		index int
		key   string
	}
	type settled struct {
		key     string
		status  string
		success bool
	}
	settledByIndex := make([]settled, len(keys))
	work := make(chan indexed)
	var mu sync.Mutex
	var errorsOut []BulkError
	done := 0
	report := func(key string, status string) {
		done++
		emitHook(opts.OnProgress, TransferProgress{Done: done, Total: total, Key: key, Status: status})
	}
	worker := func() {
		for job := range work {
			destKey := transform(job.key)
			status := "transferred"
			err := transferOne(ctx, source, dest, job.key, destKey, overwrite)
			mu.Lock()
			if errors.Is(err, errTransferSkipped) {
				status = "skipped"
				settledByIndex[job.index] = settled{key: job.key, status: status, success: true}
				report(job.key, status)
			} else if err != nil {
				errorsOut = append(errorsOut, BulkError{Key: job.key, Error: WrapError(err, ErrProvider)})
			} else {
				settledByIndex[job.index] = settled{key: job.key, status: status, success: true}
				report(job.key, status)
			}
			mu.Unlock()
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			worker()
		}()
	}
	for i, key := range keys {
		if opts.StopOnError {
			mu.Lock()
			hasError := len(errorsOut) > 0
			mu.Unlock()
			if hasError {
				break
			}
		}
		work <- indexed{index: i, key: key}
	}
	close(work)
	wg.Wait()
	out := TransferResult{Errors: errorsOut}
	for _, item := range settledByIndex {
		if !item.success {
			continue
		}
		if item.status == "skipped" {
			out.Skipped = append(out.Skipped, item.key)
		} else {
			out.Transferred = append(out.Transferred, item.key)
		}
	}
	return out, nil
}

func transferOne(ctx context.Context, source *Client, dest *Client, sourceKey string, destKey string, overwrite bool) error {
	if !overwrite {
		exists, err := dest.Exists(ctx, destKey, OperationOptions{})
		if err != nil {
			return err
		}
		if exists {
			return errTransferSkipped
		}
	}
	file, err := source.Download(ctx, sourceKey, DownloadOptions{})
	if err != nil {
		return err
	}
	reader, err := file.Open(ctx)
	if err != nil {
		return err
	}
	defer reader.Close()
	_, err = dest.Upload(ctx, destKey, ReaderBody(reader), UploadOptions{
		ContentType: file.ContentType,
		Metadata:    cloneStringMap(file.Metadata),
	})
	return err
}

var errTransferSkipped = errors.New("transfer skipped")
