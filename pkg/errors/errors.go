package errors

import (
	"fmt"
	"net/http"
)

type AppError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *AppError) Error() string {
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

func NewAppError(code int, msg string) *AppError {
	return &AppError{Code: code, Message: msg}
}

func RateLimitError() *AppError {
	return NewAppError(http.StatusTooManyRequests, "rate limit exceeded")
}

func ValidationError(msg string) *AppError {
	return NewAppError(http.StatusBadRequest, msg)
}

func NotFoundError(resource string) *AppError {
	return NewAppError(http.StatusNotFound, resource+" not found")
}

func ConflictError(msg string) *AppError {
	return NewAppError(http.StatusConflict, msg)
}

func InternalError(msg string) *AppError {
	return NewAppError(http.StatusInternalServerError, msg)
}
