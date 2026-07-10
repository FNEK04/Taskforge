package errors_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	apperrors "github.com/taskforge/pkg/errors"
)

func TestRateLimitError(t *testing.T) {
	err := apperrors.RateLimitError()
	assert.Equal(t, http.StatusTooManyRequests, err.Code)
	assert.Equal(t, "rate limit exceeded", err.Message)
}

func TestValidationError(t *testing.T) {
	err := apperrors.ValidationError("bad input")
	assert.Equal(t, http.StatusBadRequest, err.Code)
	assert.Equal(t, "bad input", err.Message)
}

func TestNotFoundError(t *testing.T) {
	err := apperrors.NotFoundError("job")
	assert.Equal(t, http.StatusNotFound, err.Code)
	assert.Equal(t, "job not found", err.Message)
}

func TestConflictError(t *testing.T) {
	err := apperrors.ConflictError("duplicate key")
	assert.Equal(t, http.StatusConflict, err.Code)
	assert.Equal(t, "duplicate key", err.Message)
}

func TestInternalError(t *testing.T) {
	err := apperrors.InternalError("db down")
	assert.Equal(t, http.StatusInternalServerError, err.Code)
	assert.Equal(t, "db down", err.Message)
}

func TestAppError_Error(t *testing.T) {
	err := apperrors.NewAppError(http.StatusTeapot, "short and stout")
	assert.Equal(t, "[418] short and stout", err.Error())
}

func TestAppError_ImplementsError(t *testing.T) {
	var e error = apperrors.NewAppError(400, "x")
	_ = e
}
