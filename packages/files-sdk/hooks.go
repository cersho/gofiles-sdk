package files

import "time"

type ActionType string

const (
	ActionUpload          ActionType = "upload"
	ActionDownload        ActionType = "download"
	ActionHead            ActionType = "head"
	ActionExists          ActionType = "exists"
	ActionDelete          ActionType = "delete"
	ActionCopy            ActionType = "copy"
	ActionMove            ActionType = "move"
	ActionList            ActionType = "list"
	ActionURL             ActionType = "url"
	ActionSignedUploadURL ActionType = "signedUploadUrl"
)

type ActionEvent struct {
	Type     ActionType
	Key      string
	Keys     []string
	From     string
	To       string
	Duration time.Duration
	Status   string
	Result   any
	Error    *Error
}

type ErrorEvent struct {
	Type     ActionType
	Key      string
	Keys     []string
	From     string
	To       string
	Duration time.Duration
	Error    *Error
}

type RetryEvent struct {
	Type       ActionType
	Key        string
	From       string
	To         string
	Attempt    int
	MaxRetries int
	Delay      time.Duration
	Error      *Error
}

type Hooks struct {
	OnAction func(ActionEvent)
	OnError  func(ErrorEvent)
	OnRetry  func(RetryEvent)
}

type actionContext struct {
	Type ActionType
	Key  string
	Keys []string
	From string
	To   string
}

func emitHook[T any](hook func(T), event T) {
	if hook == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	hook(event)
}
