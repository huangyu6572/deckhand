package wire

import "fmt"

type Error struct {
	Code      string
	Message   string
	Retryable bool
	Status    string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return e.Code
	}
	return e.Code + ": " + e.Message
}

func E(code, msg string) *Error {
	return &Error{Code: code, Message: msg, Retryable: retryable(code), Status: failStatus(code)}
}

func Ef(code, format string, args ...any) *Error {
	return E(code, fmt.Sprintf(format, args...))
}

func retryable(code string) bool {
	switch code {
	case "REMOTE_UNREACHABLE", "JOB_TIMEOUT", "EXECUTION_UNKNOWN", "SESSION_BUSY",
		"OUTPUT_PERSIST_FAILED", "LOCAL_IO_ERROR", "STORAGE_ERROR", "SERIAL_BUSY",
		"SERIAL_DISCONNECTED", "SERIAL_TIMEOUT", "FILE_CHANGED", "DAEMON_INSTANCE_CONFLICT":
		return true
	default:
		return false
	}
}

func failStatus(code string) string {
	switch code {
	case "JOB_TIMEOUT":
		return "timed_out"
	case "JOB_CANCELLED":
		return "cancelled"
	case "EXECUTION_UNKNOWN":
		return "execution_unknown"
	default:
		return "failed"
	}
}

func ExitCode(code string) int {
	switch code {
	case "INVALID_ARGUMENT", "CONFIG_INVALID", "RECIPE_INVALID", "RECIPE_NOT_FOUND", "OPENSSH_UNSUPPORTED", "DESTROY_NEEDS_HUMAN":
		return 2
	case "IPC_PROTOCOL_ERROR", "DAEMON_INSTANCE_CONFLICT":
		return 3
	default:
		return 1
	}
}

func Is(err error, code string) bool {
	e, ok := err.(*Error)
	return ok && e.Code == code
}
