package errors

import (
	"reflect"
	"strings"

	"liuproxy_go/internal/xray_core/common/serial"
)

const trim = len("liuproxy_go/internal/xray_core/")

type hasInnerError interface {
	// Unwrap returns the underlying error of this one.
	Unwrap() error
}

// Error is an error object with underlying error.
type Error struct {
	pathObj interface{}
	prefix  []interface{}
	message []interface{}
	inner   error
	//severity log.Severity
}

func (err *Error) WithPathObj(obj interface{}) *Error {
	err.pathObj = obj
	return err
}

func (err *Error) pkgPath() string {
	if err.pathObj == nil {
		return ""
	}
	path := reflect.TypeOf(err.pathObj).PkgPath()
	if len(path) >= trim {
		return path[trim:]
	}
	return path
}

// Error implements error.Error().
func (err *Error) Error() string {
	builder := strings.Builder{}
	for _, prefix := range err.prefix {
		builder.WriteByte('[')
		builder.WriteString(serial.ToString(prefix))
		builder.WriteString("] ")
	}

	path := err.pkgPath()
	if len(path) > 0 {
		builder.WriteString(path)
		builder.WriteString(": ")
	}

	msg := serial.Concat(err.message...)
	builder.WriteString(msg)

	if err.inner != nil {
		builder.WriteString(" > ")
		builder.WriteString(err.inner.Error())
	}

	return builder.String()
}

// Unwrap implements hasInnerError.Unwrap()
func (err *Error) Unwrap() error {
	if err.inner == nil {
		return nil
	}
	return err.inner
}

func (err *Error) Base(e error) *Error {
	err.inner = e
	return err
}

// String returns the string representation of this error.
func (err *Error) String() string {
	return err.Error()
}

// New returns a new error object with message formed from given arguments.
func New(msg ...interface{}) *Error {
	return &Error{
		message: msg,
		//severity: log.Severity_Info,
	}
}

// Cause returns the root cause of this error.
func Cause(err error) error {
	if err == nil {
		return nil
	}
L:
	for {
		switch inner := err.(type) {
		case hasInnerError:
			if inner.Unwrap() == nil {
				break L
			}
			err = inner.Unwrap()
		default:
			break L
		}
	}
	return err
}

// GetSeverity returns the actual severity of the error, including inner errors.
//func GetSeverity(err error) log.Severity {
//	if s, ok := err.(hasSeverity); ok {
//		return s.Severity()
//	}
//	return log.Severity_Info
//}
