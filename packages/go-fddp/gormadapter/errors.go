package gormadapter

import "fmt"

type ErrorCode string

const (
	ErrorInvalidConfig     ErrorCode = "INVALID_CONFIG"
	ErrorFieldNotMapped    ErrorCode = "FIELD_NOT_MAPPED"
	ErrorRelationNotMapped ErrorCode = "RELATION_NOT_MAPPED"
	ErrorUnsafeIdentifier  ErrorCode = "UNSAFE_IDENTIFIER"
	ErrorUnsupportedFilter ErrorCode = "UNSUPPORTED_FILTER"
	ErrorInvalidCursor     ErrorCode = "INVALID_CURSOR"
	ErrorUnsupportedExpand ErrorCode = "UNSUPPORTED_EXPAND"
	ErrorProjectionFailed  ErrorCode = "PROJECTION_FAILED"
)

type AdapterError struct {
	Code  ErrorCode
	What  string
	Hint  string
	Cause error
}

func (err *AdapterError) Error() string {
	if err == nil {
		return ""
	}
	message := fmt.Sprintf("fddp gormadapter: [%s] %s", err.Code, err.What)
	if err.Hint != "" {
		message += "; hint: " + err.Hint
	}
	return message
}

func (err *AdapterError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.Cause
}

func (err *AdapterError) ErrorCode() string {
	if err == nil {
		return ""
	}
	return string(err.Code)
}

func (err *AdapterError) ErrorHint() string {
	if err == nil {
		return ""
	}
	return err.Hint
}

func adapterError(code ErrorCode, what string, hint string) error {
	return &AdapterError{Code: code, What: what, Hint: hint}
}

func wrapAdapterError(code ErrorCode, what string, hint string, cause error) error {
	return &AdapterError{Code: code, What: what, Hint: hint, Cause: cause}
}
