package sailor

import "fmt"

// Code is a stable, machine-readable error identifier (SAILOR_CONSUMER.md section 14).
type Code string

const (
	CodeEmptyResources        Code = "EMPTY_RESOURCES"
	CodeEmptyResourceName     Code = "EMPTY_RESOURCE_NAME"
	CodeNoConnection          Code = "NO_CONNECTION"
	CodeInvalidURIPrefix      Code = "INVALID_URI_PREFIX"
	CodeMissingAccessKey      Code = "MISSING_ACCESS_KEY"
	CodeMissingSecretKey      Code = "MISSING_SECRET_KEY"
	CodeMissingPathComponents Code = "MISSING_PATH_COMPONENTS"
	CodeInvalidScheme         Code = "INVALID_SCHEME"
	CodeMissingNamespaceApp   Code = "MISSING_NAMESPACE_APP"
	CodeDuplicateResource     Code = "DUPLICATE_RESOURCE"
	CodeInvalidPullInterval   Code = "INVALID_PULL_INTERVAL"
	CodeUnsupportedLang       Code = "UNSUPPORTED_LANG"
	CodeLocalConfigNotFound   Code = "LOCAL_CONFIG_NOT_FOUND"
	CodeLocalConfigInvalid    Code = "LOCAL_CONFIG_INVALID"
	CodeLocalConfigEnvMissing Code = "LOCAL_CONFIG_ENV_NOT_FOUND"
	CodeResourceNotLoaded     Code = "RESOURCE_NOT_LOADED"
	CodeFallbackFailed        Code = "FALLBACK_FAILED"
	CodeTypeMismatch          Code = "TYPE_MISMATCH"
	CodeDecodeFailed          Code = "DECODE_FAILED"
	CodeDecryptFailed         Code = "DECRYPT_FAILED"
	CodeInvalidURI            Code = "INVALID_URI"
	CodeAlreadyStarted        Code = "ALREADY_STARTED"
	CodeInvalidFetchMode      Code = "INVALID_FETCH_MODE"
)

// Error is returned by the consumer. Compare with errors.Is against the
// sentinel for a code; matching is by Code only.
type Error struct {
	Code Code
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("sailor: %s: %s: %v", e.Code, e.Msg, e.Err)
	}
	return fmt.Sprintf("sailor: %s: %s", e.Code, e.Msg)
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

func errf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

func wrapf(code Code, err error, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...), Err: err}
}

var (
	ErrEmptyResources        = &Error{Code: CodeEmptyResources}
	ErrEmptyResourceName     = &Error{Code: CodeEmptyResourceName}
	ErrNoConnection          = &Error{Code: CodeNoConnection}
	ErrInvalidURIPrefix      = &Error{Code: CodeInvalidURIPrefix}
	ErrMissingAccessKey      = &Error{Code: CodeMissingAccessKey}
	ErrMissingSecretKey      = &Error{Code: CodeMissingSecretKey}
	ErrMissingPathComponents = &Error{Code: CodeMissingPathComponents}
	ErrInvalidScheme         = &Error{Code: CodeInvalidScheme}
	ErrMissingNamespaceApp   = &Error{Code: CodeMissingNamespaceApp}
	ErrDuplicateResource     = &Error{Code: CodeDuplicateResource}
	ErrInvalidPullInterval   = &Error{Code: CodeInvalidPullInterval}
	ErrUnsupportedLang       = &Error{Code: CodeUnsupportedLang}
	ErrLocalConfigNotFound   = &Error{Code: CodeLocalConfigNotFound}
	ErrLocalConfigInvalid    = &Error{Code: CodeLocalConfigInvalid}
	ErrLocalConfigEnvMissing = &Error{Code: CodeLocalConfigEnvMissing}
	ErrResourceNotLoaded     = &Error{Code: CodeResourceNotLoaded}
	ErrFallbackFailed        = &Error{Code: CodeFallbackFailed}
	ErrTypeMismatch          = &Error{Code: CodeTypeMismatch}
	ErrDecodeFailed          = &Error{Code: CodeDecodeFailed}
	ErrDecryptFailed         = &Error{Code: CodeDecryptFailed}
)
