package files

import (
	"context"
	"time"
)

const DefaultURLExpiresIn = time.Hour

type RetryBackoffContext struct {
	Attempt int
	Error   *Error
}

type RetryOptions struct {
	Max     int
	Backoff func(RetryBackoffContext) time.Duration
}

type OperationOptions struct {
	Timeout time.Duration
	Retries *RetryOptions
}

type UploadProgress struct {
	Loaded int64
	Total  int64
	Known  bool
}

type MultipartOptions struct {
	PartSize    int64
	Concurrency int
}

type UploadOptions struct {
	OperationOptions
	ContentType  string
	CacheControl string
	Metadata     map[string]string
	OnProgress   func(UploadProgress)
	Multipart    *MultipartOptions
	Control      *UploadControl
}

type UploadResult struct {
	Key          string
	Size         int64
	ContentType  string
	ETag         string
	LastModified time.Time
}

type ByteRange struct {
	Start int64
	End   *int64
}

type DownloadOptions struct {
	OperationOptions
	Range *ByteRange
}

type ListOptions struct {
	OperationOptions
	Prefix    string
	Cursor    string
	Limit     int32
	Delimiter string
}

type ListResult struct {
	Items    []StoredFile
	Prefixes []string
	Cursor   string
}

type SearchMatch string

const (
	SearchGlob      SearchMatch = "glob"
	SearchRegex     SearchMatch = "regex"
	SearchSubstring SearchMatch = "substring"
	SearchExact     SearchMatch = "exact"
)

type SearchOptions struct {
	OperationOptions
	Prefix          string
	Limit           int32
	MaxResults      int
	Match           SearchMatch
	CaseInsensitive bool
}

type DeleteManyOptions struct {
	Concurrency int
	StopOnError bool
}

type DeleteManyError struct {
	Key   string
	Error *Error
}

type DeleteManyResult struct {
	Deleted []string
	Errors  []DeleteManyError
}

type BulkOptions struct {
	Concurrency int
	StopOnError bool
}

type BulkError struct {
	Key   string
	Error *Error
}

type UploadManyItem struct {
	Key          string
	Body         Body
	ContentType  string
	CacheControl string
	Metadata     map[string]string
	Multipart    *MultipartOptions
}

type UploadManyOptions struct {
	BulkOptions
	OnProgress func(string, UploadProgress)
}

type UploadManyResult struct {
	Uploaded []UploadResult
	Errors   []BulkError
}

type DownloadManyOptions struct {
	BulkOptions
	Range *ByteRange
}

type DownloadManyResult struct {
	Downloaded []StoredFile
	Errors     []BulkError
}

type HeadManyResult struct {
	Files  []StoredFile
	Errors []BulkError
}

type ExistsManyResult struct {
	Existing []string
	Missing  []string
	Errors   []BulkError
}

type URLOptions struct {
	OperationOptions
	ExpiresIn                  time.Duration
	ResponseContentDisposition string
}

type SignedUploadOptions struct {
	OperationOptions
	ExpiresIn   time.Duration
	ContentType string
	MaxSize     *int64
	MinSize     *int64
}

type SignedUpload struct {
	Method  string
	URL     string
	Headers map[string]string
	Fields  map[string]string
}

type SignedURLCapability struct {
	Supported    bool
	MaxExpiresIn time.Duration
}

type AdapterCapabilities struct {
	RangeRead      bool
	UploadProgress bool
	Delimiter      bool
	Metadata       bool
	CacheControl   bool
	Multipart      bool
	Resumable      bool
	ServerSideCopy bool
	SignedURL      SignedURLCapability
}

type Adapter interface {
	Name() string
	Raw() any
	Capabilities() AdapterCapabilities
	Upload(context.Context, string, Body, UploadOptions) (UploadResult, error)
	Download(context.Context, string, DownloadOptions) (StoredFile, error)
	Head(context.Context, string, OperationOptions) (StoredFile, error)
	Exists(context.Context, string, OperationOptions) (bool, error)
	Delete(context.Context, string, OperationOptions) error
	Copy(context.Context, string, string, OperationOptions) error
	List(context.Context, ListOptions) (ListResult, error)
	URL(context.Context, string, URLOptions) (string, error)
	SignedUploadURL(context.Context, string, SignedUploadOptions) (SignedUpload, error)
}

type DeleteManyAdapter interface {
	DeleteMany(context.Context, []string, DeleteManyOptions) (DeleteManyResult, error)
}

type MoveAdapter interface {
	Move(context.Context, string, string, OperationOptions) error
}
