package s3

import (
	"errors"
	"fmt"

	"github.com/aws/smithy-go"

	files "github.com/cersho/gofiles-sdk"
)

type apiError struct {
	code    string
	message string
}

func (e apiError) Error() string { return e.message }

func (e apiError) ErrorCode() string { return e.code }

func (e apiError) ErrorMessage() string { return e.message }

func (e apiError) ErrorFault() smithy.ErrorFault { return smithy.FaultUnknown }

func mapS3Error(err error, providerLabel string) *files.Error {
	if err == nil {
		return nil
	}
	var fe *files.Error
	if errors.As(err, &fe) {
		return fe
	}
	var api smithy.APIError
	if errors.As(err, &api) {
		code := api.ErrorCode()
		msg := api.ErrorMessage()
		if msg == "" {
			msg = api.Error()
		}
		switch code {
		case "NoSuchKey", "NotFound", "NoSuchBucket":
			return files.NewError(files.ErrNotFound, msg, err)
		case "AccessDenied", "Unauthorized", "InvalidAccessKeyId", "SignatureDoesNotMatch":
			return files.NewError(files.ErrUnauthorized, msg, err)
		case "PreconditionFailed", "Conflict":
			return files.NewError(files.ErrConflict, msg, err)
		default:
			if msg == "" {
				msg = providerLabel
			}
			return files.NewError(files.ErrProvider, msg, err)
		}
	}
	msg := err.Error()
	if msg == "" {
		msg = providerLabel
	}
	return files.NewError(files.ErrProvider, fmt.Sprintf("%s: %s", providerLabel, msg), err)
}
