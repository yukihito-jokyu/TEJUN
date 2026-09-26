package wails

import (
	"encoding/json"
	"errors"

	"github.com/yukihito-jokyu/TEJUN/internal/domain/shared"
)

type errorCause struct {
	Code            string            `json:"code"`
	Message         string            `json:"message"`
	FieldErrors     map[string]string `json:"fieldErrors,omitempty"`
	Retryable       bool              `json:"retryable"`
	CauseID         string            `json:"causeId,omitempty"`
	CurrentRevision *int64            `json:"currentRevision,omitempty"`
}

func MarshalError(err error) []byte {
	var appErr *shared.Error
	if !errors.As(err, &appErr) {
		appErr = &shared.Error{Code: "internal", Message: "予期しないエラーが発生しました"}
	}

	cause := errorCause{
		Code:            appErr.Code,
		Message:         appErr.Message,
		FieldErrors:     appErr.FieldErrors,
		Retryable:       appErr.Retryable,
		CauseID:         appErr.CauseID,
		CurrentRevision: appErr.CurrentRevision,
	}

	b, marshalErr := json.Marshal(cause)
	if marshalErr != nil {
		return []byte(`{"code":"internal","message":"予期しないエラーが発生しました","retryable":false}`)
	}

	return b
}
