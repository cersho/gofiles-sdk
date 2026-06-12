package s3

import (
	"bytes"
	"context"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	files "github.com/cersho/gofiles-sdk"
)

func (a *Adapter) ResumableUpload(_ context.Context, key string, opts files.ResumableUploadOptions) (files.ResumableDriver, error) {
	return &resumableDriver{adapter: a, key: key, opts: opts}, nil
}

type resumableDriver struct {
	adapter *Adapter
	key     string
	opts    files.ResumableUploadOptions
	session files.ResumableSession
}

func (d *resumableDriver) Begin(ctx context.Context, meta files.ResumableUploadMeta) (files.ResumableSession, error) {
	input := &awss3.CreateMultipartUploadInput{
		Bucket:      aws.String(d.adapter.bucket),
		Key:         aws.String(d.key),
		ContentType: aws.String(meta.ContentType),
	}
	if meta.CacheControl != "" {
		input.CacheControl = aws.String(meta.CacheControl)
	}
	if len(meta.Metadata) > 0 {
		input.Metadata = cloneStringMap(meta.Metadata)
	}
	out, err := d.adapter.client.CreateMultipartUpload(ctx, input)
	if err != nil {
		return files.ResumableSession{}, mapS3Error(err, d.adapter.providerLabel)
	}
	session := files.ResumableSession{
		Provider:    d.adapter.name,
		Bucket:      d.adapter.bucket,
		Key:         d.key,
		UploadID:    aws.ToString(out.UploadId),
		PartSize:    meta.PartSize,
		ContentType: meta.ContentType,
	}
	d.session = session
	return session, nil
}

func (d *resumableDriver) Adopt(_ context.Context, session files.ResumableSession) error {
	if session.Bucket != d.adapter.bucket || session.Key != d.key || session.UploadID == "" {
		return files.NewError(files.ErrProvider, d.adapter.name+": resumable session does not match this upload", nil)
	}
	d.session = session
	return nil
}

func (d *resumableDriver) Probe(ctx context.Context) (files.ResumableProbe, error) {
	if d.session.UploadID == "" {
		return files.ResumableProbe{}, files.NewError(files.ErrProvider, d.adapter.name+": resumable session is not initialized", nil)
	}
	var parts []files.PartMeta
	var marker *string
	for {
		out, err := d.adapter.client.ListParts(ctx, &awss3.ListPartsInput{
			Bucket:           aws.String(d.adapter.bucket),
			Key:              aws.String(d.key),
			UploadId:         aws.String(d.session.UploadID),
			PartNumberMarker: marker,
		})
		if err != nil {
			return files.ResumableProbe{}, mapS3Error(err, d.adapter.providerLabel)
		}
		for _, part := range out.Parts {
			parts = append(parts, files.PartMeta{
				PartNumber: int(aws.ToInt32(part.PartNumber)),
				Size:       aws.ToInt64(part.Size),
				ETag:       stripETag(aws.ToString(part.ETag)),
			})
		}
		if !aws.ToBool(out.IsTruncated) {
			break
		}
		marker = out.NextPartNumberMarker
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })
	var offset int64
	for _, part := range parts {
		offset += part.Size
	}
	return files.ResumableProbe{NextOffset: offset, Parts: parts}, nil
}

func (d *resumableDriver) UploadPart(ctx context.Context, part files.ResumablePart) (files.PartMeta, error) {
	out, err := d.adapter.client.UploadPart(ctx, &awss3.UploadPartInput{
		Bucket:        aws.String(d.adapter.bucket),
		Key:           aws.String(d.key),
		UploadId:      aws.String(d.session.UploadID),
		PartNumber:    aws.Int32(int32(part.PartNumber)),
		ContentLength: aws.Int64(int64(len(part.Data))),
		Body:          bytes.NewReader(part.Data),
	})
	if err != nil {
		return files.PartMeta{}, mapS3Error(err, d.adapter.providerLabel)
	}
	return files.PartMeta{PartNumber: part.PartNumber, Size: int64(len(part.Data)), ETag: stripETag(aws.ToString(out.ETag))}, nil
}

func (d *resumableDriver) Complete(ctx context.Context, parts []files.PartMeta) (files.UploadResult, error) {
	sort.Slice(parts, func(i, j int) bool { return parts[i].PartNumber < parts[j].PartNumber })
	completed := make([]types.CompletedPart, 0, len(parts))
	var size int64
	for _, part := range parts {
		completed = append(completed, types.CompletedPart{
			ETag:       aws.String(part.ETag),
			PartNumber: aws.Int32(int32(part.PartNumber)),
		})
		size += part.Size
	}
	out, err := d.adapter.client.CompleteMultipartUpload(ctx, &awss3.CompleteMultipartUploadInput{
		Bucket:          aws.String(d.adapter.bucket),
		Key:             aws.String(d.key),
		UploadId:        aws.String(d.session.UploadID),
		MultipartUpload: &types.CompletedMultipartUpload{Parts: completed},
	})
	if err != nil {
		return files.UploadResult{}, mapS3Error(err, d.adapter.providerLabel)
	}
	return files.UploadResult{
		Key:         d.key,
		Size:        size,
		ContentType: d.session.ContentType,
		ETag:        stripETag(aws.ToString(out.ETag)),
	}, nil
}

func (d *resumableDriver) Abort(ctx context.Context) error {
	if d.session.UploadID == "" {
		return nil
	}
	_, err := d.adapter.client.AbortMultipartUpload(ctx, &awss3.AbortMultipartUploadInput{
		Bucket:   aws.String(d.adapter.bucket),
		Key:      aws.String(d.key),
		UploadId: aws.String(d.session.UploadID),
	})
	if err != nil {
		return mapS3Error(err, d.adapter.providerLabel)
	}
	return nil
}
