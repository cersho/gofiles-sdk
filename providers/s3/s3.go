package s3

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	files "github.com/cersho/gofiles-sdk"
	"github.com/cersho/gofiles-sdk/internal/typeutil"
)

type Credentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
}

type Options struct {
	Bucket               string
	Region               string
	Endpoint             string
	ForcePathStyle       bool
	Credentials          *Credentials
	PublicBaseURL        string
	DefaultURLExpiresIn  time.Duration
	DefaultProviderLabel string
	Client               *awss3.Client
}

type Adapter struct {
	client              *awss3.Client
	presign             *awss3.PresignClient
	bucket              string
	publicBaseURL       string
	defaultURLExpiresIn time.Duration
	providerLabel       string
	name                string
}

func New(ctx context.Context, opts Options) (*Adapter, error) {
	return NewNamed(ctx, "s3", opts)
}

func NewNamed(ctx context.Context, name string, opts Options) (*Adapter, error) {
	if opts.Bucket == "" {
		return nil, files.NewError(files.ErrProvider, name+" adapter: missing bucket", nil)
	}
	region := opts.Region
	if region == "" {
		region = os.Getenv("AWS_REGION")
		if region == "" {
			region = os.Getenv("AWS_DEFAULT_REGION")
		}
	}
	if region == "" && opts.Client == nil {
		return nil, files.NewError(files.ErrProvider, name+" adapter: missing region", nil)
	}
	client := opts.Client
	if client == nil {
		loadOpts := []func(*config.LoadOptions) error{config.WithRegion(region)}
		if opts.Credentials != nil {
			loadOpts = append(loadOpts, config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				opts.Credentials.AccessKeyID,
				opts.Credentials.SecretAccessKey,
				opts.Credentials.SessionToken,
			)))
		}
		cfg, err := config.LoadDefaultConfig(ctx, loadOpts...)
		if err != nil {
			return nil, mapS3Error(err, opts.DefaultProviderLabel)
		}
		client = awss3.NewFromConfig(cfg, func(o *awss3.Options) {
			if opts.Endpoint != "" {
				o.BaseEndpoint = aws.String(opts.Endpoint)
			}
			o.UsePathStyle = opts.ForcePathStyle
		})
	}
	expires := opts.DefaultURLExpiresIn
	if expires <= 0 {
		expires = files.DefaultURLExpiresIn
	}
	label := opts.DefaultProviderLabel
	if label == "" {
		label = "S3 error"
	}
	return &Adapter{
		client:              client,
		presign:             awss3.NewPresignClient(client),
		bucket:              opts.Bucket,
		publicBaseURL:       opts.PublicBaseURL,
		defaultURLExpiresIn: expires,
		providerLabel:       label,
		name:                name,
	}, nil
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Raw() any { return a.client }

func (a *Adapter) Bucket() string { return a.bucket }

func (a *Adapter) Capabilities() files.AdapterCapabilities {
	return files.AdapterCapabilities{
		RangeRead:      true,
		UploadProgress: true,
		Delimiter:      true,
		Metadata:       true,
		CacheControl:   true,
		Multipart:      true,
		Resumable:      true,
		ServerSideCopy: true,
		SignedURL:      files.SignedURLCapability{Supported: true},
	}
}

func (a *Adapter) Upload(ctx context.Context, key string, body files.Body, opts files.UploadOptions) (files.UploadResult, error) {
	contentType := typeutil.EffectiveContentType(opts.ContentType, body.ContentType())
	uploadBody := body
	if opts.OnProgress != nil {
		uploadBody = files.BodyWithProgress(body, opts.OnProgress)
	}
	reader, err := uploadBody.Open(ctx)
	if err != nil {
		return files.UploadResult{}, err
	}
	defer reader.Close()
	size, sizeKnown := uploadBody.Size()
	input := &awss3.PutObjectInput{
		Bucket:      aws.String(a.bucket),
		Key:         aws.String(key),
		Body:        reader,
		ContentType: aws.String(contentType),
	}
	if opts.CacheControl != "" {
		input.CacheControl = aws.String(opts.CacheControl)
	}
	if len(opts.Metadata) > 0 {
		input.Metadata = cloneStringMap(opts.Metadata)
	}
	if sizeKnown {
		input.ContentLength = aws.Int64(size)
	}
	var etag string
	if opts.Multipart != nil {
		uploader := manager.NewUploader(a.client, func(u *manager.Uploader) {
			if opts.Multipart.PartSize > 0 {
				u.PartSize = opts.Multipart.PartSize
			}
			if opts.Multipart.Concurrency > 0 {
				u.Concurrency = opts.Multipart.Concurrency
			}
		})
		out, err := uploader.Upload(ctx, input)
		if err != nil {
			return files.UploadResult{}, mapS3Error(err, a.providerLabel)
		}
		etag = stripETag(aws.ToString(out.ETag))
	} else {
		out, err := a.client.PutObject(ctx, input)
		if err != nil {
			return files.UploadResult{}, mapS3Error(err, a.providerLabel)
		}
		etag = stripETag(aws.ToString(out.ETag))
	}
	lastModified := time.Time{}
	if !sizeKnown {
		head, err := a.client.HeadObject(ctx, &awss3.HeadObjectInput{
			Bucket: aws.String(a.bucket),
			Key:    aws.String(key),
		})
		if err == nil {
			size = aws.ToInt64(head.ContentLength)
			lastModified = aws.ToTime(head.LastModified)
		}
	}
	return files.UploadResult{
		Key:          key,
		Size:         size,
		ContentType:  contentType,
		ETag:         etag,
		LastModified: lastModified,
	}, nil
}

func (a *Adapter) Download(ctx context.Context, key string, opts files.DownloadOptions) (files.StoredFile, error) {
	input := &awss3.GetObjectInput{Bucket: aws.String(a.bucket), Key: aws.String(key)}
	if opts.Range != nil {
		input.Range = aws.String(rangeHeader(*opts.Range))
	}
	out, err := a.client.GetObject(ctx, input)
	if err != nil {
		return files.StoredFile{}, mapS3Error(err, a.providerLabel)
	}
	meta := files.StoredFileMeta{
		Key:          key,
		Size:         aws.ToInt64(out.ContentLength),
		ContentType:  aws.ToString(out.ContentType),
		LastModified: aws.ToTime(out.LastModified),
		ETag:         stripETag(aws.ToString(out.ETag)),
		Metadata:     out.Metadata,
	}
	return files.NewStoredFile(meta, func(context.Context) (io.ReadCloser, error) {
		return out.Body, nil
	}), nil
}

func (a *Adapter) Head(ctx context.Context, key string, _ files.OperationOptions) (files.StoredFile, error) {
	out, err := a.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return files.StoredFile{}, mapS3Error(err, a.providerLabel)
	}
	meta := files.StoredFileMeta{
		Key:          key,
		Size:         aws.ToInt64(out.ContentLength),
		ContentType:  aws.ToString(out.ContentType),
		LastModified: aws.ToTime(out.LastModified),
		ETag:         stripETag(aws.ToString(out.ETag)),
		Metadata:     out.Metadata,
	}
	return files.NewStoredFile(meta, func(ctx context.Context) (io.ReadCloser, error) {
		get, err := a.client.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(a.bucket), Key: aws.String(key)})
		if err != nil {
			return nil, mapS3Error(err, a.providerLabel)
		}
		return get.Body, nil
	}), nil
}

func (a *Adapter) Exists(ctx context.Context, key string, opts files.OperationOptions) (bool, error) {
	_, err := a.Head(ctx, key, opts)
	if err == nil {
		return true, nil
	}
	if files.IsCode(err, files.ErrNotFound) {
		return false, nil
	}
	return false, err
}

func (a *Adapter) Delete(ctx context.Context, key string, _ files.OperationOptions) error {
	_, err := a.client.DeleteObject(ctx, &awss3.DeleteObjectInput{
		Bucket: aws.String(a.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return mapS3Error(err, a.providerLabel)
	}
	return nil
}

func (a *Adapter) DeleteMany(ctx context.Context, keys []string, opts files.DeleteManyOptions) (files.DeleteManyResult, error) {
	if len(keys) == 0 {
		return files.DeleteManyResult{}, nil
	}
	if opts.StopOnError {
		out := files.DeleteManyResult{}
		for _, key := range keys {
			if err := a.Delete(ctx, key, files.OperationOptions{}); err != nil {
				out.Errors = append(out.Errors, files.DeleteManyError{Key: key, Error: files.WrapError(err, files.ErrProvider)})
				return out, nil
			}
			out.Deleted = append(out.Deleted, key)
		}
		return out, nil
	}
	out := files.DeleteManyResult{}
	const limit = 1000
	for start := 0; start < len(keys); start += limit {
		end := start + limit
		if end > len(keys) {
			end = len(keys)
		}
		batch := keys[start:end]
		objects := make([]types.ObjectIdentifier, 0, len(batch))
		for _, key := range batch {
			objects = append(objects, types.ObjectIdentifier{Key: aws.String(key)})
		}
		result, err := a.client.DeleteObjects(ctx, &awss3.DeleteObjectsInput{
			Bucket: aws.String(a.bucket),
			Delete: &types.Delete{Objects: objects},
		})
		if err != nil {
			mapped := mapS3Error(err, a.providerLabel)
			for _, key := range batch {
				out.Errors = append(out.Errors, files.DeleteManyError{Key: key, Error: mapped})
			}
			continue
		}
		for _, deleted := range result.Deleted {
			out.Deleted = append(out.Deleted, aws.ToString(deleted.Key))
		}
		for _, item := range result.Errors {
			out.Errors = append(out.Errors, files.DeleteManyError{
				Key: aws.ToString(item.Key),
				Error: mapS3Error(apiError{
					code:    aws.ToString(item.Code),
					message: aws.ToString(item.Message),
				}, a.providerLabel),
			})
		}
	}
	return out, nil
}

func (a *Adapter) Copy(ctx context.Context, from string, to string, _ files.OperationOptions) error {
	source := url.PathEscape(a.bucket) + "/" + url.PathEscape(from)
	_, err := a.client.CopyObject(ctx, &awss3.CopyObjectInput{
		Bucket:     aws.String(a.bucket),
		CopySource: aws.String(source),
		Key:        aws.String(to),
	})
	if err != nil {
		return mapS3Error(err, a.providerLabel)
	}
	return nil
}

func (a *Adapter) List(ctx context.Context, opts files.ListOptions) (files.ListResult, error) {
	input := &awss3.ListObjectsV2Input{Bucket: aws.String(a.bucket)}
	if opts.Prefix != "" {
		input.Prefix = aws.String(opts.Prefix)
	}
	if opts.Cursor != "" {
		input.ContinuationToken = aws.String(opts.Cursor)
	}
	if opts.Limit > 0 {
		input.MaxKeys = aws.Int32(opts.Limit)
	}
	if opts.Delimiter != "" {
		input.Delimiter = aws.String(opts.Delimiter)
	}
	out, err := a.client.ListObjectsV2(ctx, input)
	if err != nil {
		return files.ListResult{}, mapS3Error(err, a.providerLabel)
	}
	result := files.ListResult{}
	if aws.ToBool(out.IsTruncated) {
		result.Cursor = aws.ToString(out.NextContinuationToken)
	}
	for _, item := range out.Contents {
		key := aws.ToString(item.Key)
		result.Items = append(result.Items, files.NewStoredFile(files.StoredFileMeta{
			Key:          key,
			Size:         aws.ToInt64(item.Size),
			ContentType:  "application/octet-stream",
			LastModified: aws.ToTime(item.LastModified),
			ETag:         stripETag(aws.ToString(item.ETag)),
		}, func(ctx context.Context) (io.ReadCloser, error) {
			get, err := a.client.GetObject(ctx, &awss3.GetObjectInput{Bucket: aws.String(a.bucket), Key: aws.String(key)})
			if err != nil {
				return nil, mapS3Error(err, a.providerLabel)
			}
			return get.Body, nil
		}))
	}
	for _, prefix := range out.CommonPrefixes {
		if prefix.Prefix != nil {
			result.Prefixes = append(result.Prefixes, aws.ToString(prefix.Prefix))
		}
	}
	return result, nil
}

func (a *Adapter) URL(ctx context.Context, key string, opts files.URLOptions) (string, error) {
	if a.publicBaseURL != "" && opts.ResponseContentDisposition == "" {
		return joinPublicURL(a.publicBaseURL, key), nil
	}
	expires := opts.ExpiresIn
	if expires <= 0 {
		expires = a.defaultURLExpiresIn
	}
	input := &awss3.GetObjectInput{Bucket: aws.String(a.bucket), Key: aws.String(key)}
	if opts.ResponseContentDisposition != "" {
		input.ResponseContentDisposition = aws.String(opts.ResponseContentDisposition)
	}
	out, err := a.presign.PresignGetObject(ctx, input, func(o *awss3.PresignOptions) {
		o.Expires = expires
	})
	if err != nil {
		return "", mapS3Error(err, a.providerLabel)
	}
	return out.URL, nil
}

func (a *Adapter) SignedUploadURL(ctx context.Context, key string, opts files.SignedUploadOptions) (files.SignedUpload, error) {
	expires := opts.ExpiresIn
	if expires <= 0 {
		expires = a.defaultURLExpiresIn
	}
	input := &awss3.PutObjectInput{Bucket: aws.String(a.bucket), Key: aws.String(key)}
	if opts.ContentType != "" {
		input.ContentType = aws.String(opts.ContentType)
	}
	if opts.MaxSize != nil {
		minSize := int64(1)
		if opts.MinSize != nil {
			minSize = *opts.MinSize
		}
		conditions := []any{[]any{"content-length-range", minSize, *opts.MaxSize}}
		if opts.ContentType != "" {
			conditions = append(conditions, []any{"eq", "$Content-Type", opts.ContentType})
		}
		post, err := a.presign.PresignPostObject(ctx, input, func(o *awss3.PresignPostOptions) {
			o.Expires = expires
			o.Conditions = conditions
		})
		if err != nil {
			return files.SignedUpload{}, mapS3Error(err, a.providerLabel)
		}
		return files.SignedUpload{Method: http.MethodPost, URL: post.URL, Fields: post.Values}, nil
	}
	out, err := a.presign.PresignPutObject(ctx, input, func(o *awss3.PresignOptions) {
		o.Expires = expires
	})
	if err != nil {
		return files.SignedUpload{}, mapS3Error(err, a.providerLabel)
	}
	return files.SignedUpload{Method: http.MethodPut, URL: out.URL, Headers: headerMap(out.SignedHeader)}, nil
}
