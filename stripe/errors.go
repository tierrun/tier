package stripe

func IsInvalidRequest(err error) bool {
	return isErrorType(err, "invalid_request_error")
}

func isErrorType(err error, typ string) bool {
	if err == nil {
		return false
	}
	if se, ok := err.(*Error); ok {
		return se.Type == typ
	}
	return false
}
