package core

type ApplicationError struct {
	StatusCode int    `json:"-" bson:"statusCode"`
	Ambit      string `json:"ambit"`
	Code       string `json:"code"`
	Message    string `json:"message"`
}

func (m *ApplicationError) Error() string {
	return m.Message
}
func (m *ApplicationError) GetStatus() int {
	return m.StatusCode
}

func TechnicalErrorWithError(err error) *ApplicationError {

	return &ApplicationError{
		StatusCode: 500,
		Message:    err.Error(),
		Ambit:      AppName,
		Code:       "TECH500",
	}
}

func (m *ApplicationError) IsTechnicalError() bool {

	return m.StatusCode == 500
}

func (m *ApplicationError) IsBusinessError() bool {

	return m.StatusCode == 422
}

func TechnicalErrorWithCodeAndMessage(code, message string) *ApplicationError {

	return &ApplicationError{
		StatusCode: 500,
		Message:    message,
		Ambit:      AppName,
		Code:       code,
	}
}

func NotFoundError() *ApplicationError {
	return &ApplicationError{
		StatusCode: 404,
		Ambit:      AppName,
		Code:       "NOT-FOUND",
		Message:    "Oggetto non trovato",
	}
}

func BusinessErrorWithError(err error) *ApplicationError {

	return &ApplicationError{
		StatusCode: 422,
		Message:    err.Error(),
		Ambit:      AppName,
		Code:       "BUS422",
	}
}

func BusinessErrorWithCodeAndMessage(code, message string) *ApplicationError {

	return &ApplicationError{
		StatusCode: 422,
		Message:    message,
		Ambit:      AppName,
		Code:       code,
	}
}
