package files

import (
	"context"
	"errors"
	"fmt"
)

type ErrorCode string

const (
	ErrNotFound     ErrorCode = "NotFound"
	ErrUnauthorized ErrorCode = "Unauthorized"
	ErrConflict     ErrorCode = "Conflict"
	ErrReadOnly     ErrorCode = "ReadOnly"
	ErrProvider     ErrorCode = "Provider"
)

type Error struct {
	Code      ErrorCode
	Message   string
	Cause     error
	Aborted   bool
	TimedOut  bool
	Permanent bool
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message == "" {
		return string(e.Code)
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func NewError(code ErrorCode, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

func NewPermanentError(code ErrorCode, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause, Permanent: true}
}

func WrapError(err error, fallback ErrorCode) *Error {
	if err == nil {
		return nil
	}
	var fe *Error
	if errors.As(err, &fe) {
		return fe
	}
	if errors.Is(err, context.Canceled) {
		return &Error{
			Code:    ErrProvider,
			Message: "operation aborted",
			Cause:   err,
			Aborted: true,
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &Error{
			Code:     ErrProvider,
			Message:  "operation timed out",
			Cause:    err,
			Aborted:  true,
			TimedOut: true,
		}
	}
	return &Error{Code: fallback, Message: err.Error(), Cause: err}
}

func TimeoutError(timeoutLabel string) *Error {
	msg := "operation timed out"
	if timeoutLabel != "" {
		msg = fmt.Sprintf("operation timed out after %s", timeoutLabel)
	}
	return &Error{Code: ErrProvider, Message: msg, Aborted: true, TimedOut: true}
}

func IsCode(err error, code ErrorCode) bool {
	var fe *Error
	return errors.As(err, &fe) && fe.Code == code
}
