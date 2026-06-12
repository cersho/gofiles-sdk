package files

import (
	"context"
	"strings"
)

func (c *Client) perform(ctx context.Context, op Operation) (any, error) {
	switch op.Kind {
	case OperationUpload:
		return c.runUpload(ctx, op.Key, op.Body, op.UploadOptions, actionContext{Type: ActionUpload, Key: op.Key})
	case OperationDownload:
		path, err := c.path(op.Key, "key")
		if err != nil {
			return nil, err
		}
		if err := c.assertRangeSupported(op.DownloadOptions.Range); err != nil {
			return nil, err
		}
		result, err := c.run(ctx, op.DownloadOptions.OperationOptions, true, actionContext{Type: ActionDownload, Key: op.Key}, func(ctx context.Context) (any, error) {
			return c.adapter.Download(ctx, path, op.DownloadOptions)
		})
		if err != nil {
			return nil, err
		}
		return c.storedFile(result.(StoredFile)), nil
	case OperationHead:
		path, err := c.path(op.Key, "key")
		if err != nil {
			return nil, err
		}
		result, err := c.run(ctx, op.OperationOptions, true, actionContext{Type: ActionHead, Key: op.Key}, func(ctx context.Context) (any, error) {
			return c.adapter.Head(ctx, path, op.OperationOptions)
		})
		if err != nil {
			return nil, err
		}
		return c.storedFile(result.(StoredFile)), nil
	case OperationExists:
		path, err := c.path(op.Key, "key")
		if err != nil {
			return nil, err
		}
		return c.run(ctx, op.OperationOptions, true, actionContext{Type: ActionExists, Key: op.Key}, func(ctx context.Context) (any, error) {
			return c.adapter.Exists(ctx, path, op.OperationOptions)
		})
	case OperationDelete:
		path, err := c.path(op.Key, "key")
		if err != nil {
			return nil, err
		}
		return c.run(ctx, op.OperationOptions, true, actionContext{Type: ActionDelete, Key: op.Key}, func(ctx context.Context) (any, error) {
			return nil, c.adapter.Delete(ctx, path, op.OperationOptions)
		})
	case OperationCopy:
		from, err := c.path(op.From, "copy source")
		if err != nil {
			return nil, err
		}
		to, err := c.path(op.To, "copy destination")
		if err != nil {
			return nil, err
		}
		return c.run(ctx, op.OperationOptions, true, actionContext{Type: ActionCopy, From: op.From, To: op.To}, func(ctx context.Context) (any, error) {
			return nil, c.adapter.Copy(ctx, from, to, op.OperationOptions)
		})
	case OperationMove:
		from, err := c.path(op.From, "move source")
		if err != nil {
			return nil, err
		}
		to, err := c.path(op.To, "move destination")
		if err != nil {
			return nil, err
		}
		return c.run(ctx, op.OperationOptions, true, actionContext{Type: ActionMove, From: op.From, To: op.To}, func(ctx context.Context) (any, error) {
			return nil, c.movePath(ctx, from, to, op.OperationOptions)
		})
	case OperationList:
		return c.performList(ctx, op.ListOptions)
	case OperationURL:
		path, err := c.path(op.Key, "key")
		if err != nil {
			return nil, err
		}
		return c.run(ctx, op.URLOptions.OperationOptions, true, actionContext{Type: ActionURL, Key: op.Key}, func(ctx context.Context) (any, error) {
			return c.adapter.URL(ctx, path, op.URLOptions)
		})
	case OperationSignedUploadURL:
		path, err := c.path(op.Key, "key")
		if err != nil {
			return nil, err
		}
		return c.run(ctx, op.SignedUploadOptions.OperationOptions, true, actionContext{Type: ActionSignedUploadURL, Key: op.Key}, func(ctx context.Context) (any, error) {
			return c.adapter.SignedUploadURL(ctx, path, op.SignedUploadOptions)
		})
	default:
		return nil, NewError(ErrProvider, "unknown operation kind: "+string(op.Kind), nil)
	}
}

func (c *Client) runUpload(ctx context.Context, key string, body Body, opts UploadOptions, action actionContext) (UploadResult, error) {
	if err := c.assertUploadOptionsSupported(opts); err != nil {
		return UploadResult{}, err
	}
	path, err := c.path(key, "key")
	if err != nil {
		return UploadResult{}, err
	}
	if opts.Control != nil {
		return c.runResumableUpload(ctx, key, path, body, opts, action)
	}
	uploadBody := body
	if opts.OnProgress != nil && !c.adapter.Capabilities().UploadProgress {
		uploadBody = BodyWithProgress(body, opts.OnProgress)
		if size, ok := body.Size(); ok {
			emitHook(opts.OnProgress, UploadProgress{Loaded: 0, Total: size, Known: true})
		} else {
			emitHook(opts.OnProgress, UploadProgress{Loaded: 0})
		}
	}
	retryable := uploadBody.Replayable()
	result, err := c.run(ctx, opts.OperationOptions, retryable, action, func(ctx context.Context) (any, error) {
		return c.adapter.Upload(ctx, path, uploadBody, opts)
	})
	if err != nil {
		return UploadResult{}, err
	}
	out := c.uploadResult(result.(UploadResult))
	if opts.OnProgress != nil && !c.adapter.Capabilities().UploadProgress {
		emitHook(opts.OnProgress, UploadProgress{Loaded: out.Size, Total: out.Size, Known: true})
	}
	return out, nil
}

func (c *Client) performList(ctx context.Context, opts ListOptions) (ListResult, error) {
	if err := c.assertDelimiterSupported(opts); err != nil {
		return ListResult{}, err
	}
	listOpts := opts
	if c.prefix != "" {
		if opts.Prefix != "" {
			listOpts.Prefix = c.prefix + "/" + strings.TrimLeft(opts.Prefix, "/")
		} else {
			listOpts.Prefix = c.prefix + "/"
		}
	}
	result, err := c.run(ctx, opts.OperationOptions, true, actionContext{Type: ActionList}, func(ctx context.Context) (any, error) {
		return c.adapter.List(ctx, listOpts)
	})
	if err != nil {
		return ListResult{}, err
	}
	out := result.(ListResult)
	for i := range out.Items {
		out.Items[i] = c.storedFile(out.Items[i])
	}
	for i := range out.Prefixes {
		out.Prefixes[i] = c.stripPrefix(out.Prefixes[i])
	}
	return out, nil
}

func (c *Client) movePath(ctx context.Context, from string, to string, opts OperationOptions) error {
	if from == to {
		return nil
	}
	if adapter, ok := c.adapter.(MoveAdapter); ok {
		return adapter.Move(ctx, from, to, opts)
	}
	if err := c.adapter.Copy(ctx, from, to, opts); err != nil {
		return err
	}
	return c.adapter.Delete(ctx, from, opts)
}

func (c *Client) Upload(ctx context.Context, key string, body Body, opts UploadOptions) (UploadResult, error) {
	if err := c.assertWritable(ActionUpload); err != nil {
		return UploadResult{}, err
	}
	result, err := c.action(ctx, actionContext{Type: ActionUpload, Key: key}, func(ctx context.Context) (any, error) {
		return c.dispatch(ctx, Operation{Kind: OperationUpload, Key: key, Body: body, UploadOptions: opts}, c.perform)
	})
	if err != nil {
		return UploadResult{}, err
	}
	return result.(UploadResult), nil
}

func (c *Client) Download(ctx context.Context, key string, opts DownloadOptions) (StoredFile, error) {
	result, err := c.action(ctx, actionContext{Type: ActionDownload, Key: key}, func(ctx context.Context) (any, error) {
		return c.dispatch(ctx, Operation{Kind: OperationDownload, Key: key, DownloadOptions: opts}, c.perform)
	})
	if err != nil {
		return StoredFile{}, err
	}
	return result.(StoredFile), nil
}

func (c *Client) Head(ctx context.Context, key string, opts OperationOptions) (StoredFile, error) {
	result, err := c.action(ctx, actionContext{Type: ActionHead, Key: key}, func(ctx context.Context) (any, error) {
		return c.dispatch(ctx, Operation{Kind: OperationHead, Key: key, OperationOptions: opts}, c.perform)
	})
	if err != nil {
		return StoredFile{}, err
	}
	return result.(StoredFile), nil
}

func (c *Client) Exists(ctx context.Context, key string, opts OperationOptions) (bool, error) {
	result, err := c.action(ctx, actionContext{Type: ActionExists, Key: key}, func(ctx context.Context) (any, error) {
		return c.dispatch(ctx, Operation{Kind: OperationExists, Key: key, OperationOptions: opts}, c.perform)
	})
	if err != nil {
		return false, err
	}
	return result.(bool), nil
}

func (c *Client) Delete(ctx context.Context, key string, opts OperationOptions) error {
	if err := c.assertWritable(ActionDelete); err != nil {
		return err
	}
	_, err := c.action(ctx, actionContext{Type: ActionDelete, Key: key}, func(ctx context.Context) (any, error) {
		return c.dispatch(ctx, Operation{Kind: OperationDelete, Key: key, OperationOptions: opts}, c.perform)
	})
	return err
}

func (c *Client) Copy(ctx context.Context, from string, to string, opts OperationOptions) error {
	if err := c.assertWritable(ActionCopy); err != nil {
		return err
	}
	_, err := c.action(ctx, actionContext{Type: ActionCopy, From: from, To: to}, func(ctx context.Context) (any, error) {
		return c.dispatch(ctx, Operation{Kind: OperationCopy, From: from, To: to, OperationOptions: opts}, c.perform)
	})
	return err
}

func (c *Client) Move(ctx context.Context, from string, to string, opts OperationOptions) error {
	if err := c.assertWritable(ActionMove); err != nil {
		return err
	}
	_, err := c.action(ctx, actionContext{Type: ActionMove, From: from, To: to}, func(ctx context.Context) (any, error) {
		return c.dispatch(ctx, Operation{Kind: OperationMove, From: from, To: to, OperationOptions: opts}, c.perform)
	})
	return err
}

func (c *Client) List(ctx context.Context, opts ListOptions) (ListResult, error) {
	result, err := c.action(ctx, actionContext{Type: ActionList}, func(ctx context.Context) (any, error) {
		return c.dispatch(ctx, Operation{Kind: OperationList, ListOptions: opts}, c.perform)
	})
	if err != nil {
		return ListResult{}, err
	}
	return result.(ListResult), nil
}

func (c *Client) URL(ctx context.Context, key string, opts URLOptions) (string, error) {
	result, err := c.action(ctx, actionContext{Type: ActionURL, Key: key}, func(ctx context.Context) (any, error) {
		return c.dispatch(ctx, Operation{Kind: OperationURL, Key: key, URLOptions: opts}, c.perform)
	})
	if err != nil {
		return "", err
	}
	return result.(string), nil
}

func (c *Client) SignedUploadURL(ctx context.Context, key string, opts SignedUploadOptions) (SignedUpload, error) {
	if err := c.assertWritable(ActionSignedUploadURL); err != nil {
		return SignedUpload{}, err
	}
	result, err := c.action(ctx, actionContext{Type: ActionSignedUploadURL, Key: key}, func(ctx context.Context) (any, error) {
		return c.dispatch(ctx, Operation{Kind: OperationSignedUploadURL, Key: key, SignedUploadOptions: opts}, c.perform)
	})
	if err != nil {
		return SignedUpload{}, err
	}
	return result.(SignedUpload), nil
}
