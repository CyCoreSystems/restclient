package restclient

type Error interface {
	Error() string
	Code() int
	Message() string
}

type BaseError struct {
	StatusCode int
	Status     string
	Err        error
}

func (e BaseError) Error() string {
	return e.Err.Error()
}

func (e BaseError) Code() int {
	return e.StatusCode
}

func (e BaseError) Message() string {
	return e.Status
}
