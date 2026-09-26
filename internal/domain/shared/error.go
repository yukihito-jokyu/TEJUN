package shared

import "fmt"

type Error struct {
	Code            string
	Message         string
	FieldErrors     map[string]string
	Retryable       bool
	CauseID         string
	CurrentRevision *int64
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
