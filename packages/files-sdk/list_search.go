package files

import (
	"context"
	"errors"
)

func (c *Client) ListAll(ctx context.Context, opts ListOptions, yield func(StoredFile) error) error {
	opts.Delimiter = ""
	cursor := opts.Cursor
	for {
		opts.Cursor = cursor
		page, err := c.List(ctx, opts)
		if err != nil {
			return err
		}
		for _, item := range page.Items {
			if err := yield(item); err != nil {
				return err
			}
		}
		if page.Cursor == "" {
			return nil
		}
		cursor = page.Cursor
	}
}

func (c *Client) Search(ctx context.Context, pattern string, opts SearchOptions, yield func(StoredFile) error) error {
	if opts.MaxResults <= 0 && opts.MaxResults != 0 {
		return nil
	}
	matches, prefix, err := buildSearchMatcher(pattern, opts)
	if err != nil {
		return err
	}
	listOpts := ListOptions{
		OperationOptions: opts.OperationOptions,
		Prefix:           prefix,
		Limit:            opts.Limit,
	}
	yielded := 0
	err = c.ListAll(ctx, listOpts, func(file StoredFile) error {
		if !matches(file.Key) {
			return nil
		}
		if err := yield(file); err != nil {
			return err
		}
		yielded++
		if opts.MaxResults > 0 && yielded >= opts.MaxResults {
			return errStopSearch
		}
		return nil
	})
	if errors.Is(err, errStopSearch) {
		return nil
	}
	return err
}

var errStopSearch = errors.New("search stopped")
