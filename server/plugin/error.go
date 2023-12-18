package plugin

type BasicError struct {
	Message string
}

func NewBasicError(err error) *BasicError {
	if err == nil {
		return nil
	}

	return &BasicError{err.Error()}
}

func (e *BasicError) Error() string {
	return e.Message
}
