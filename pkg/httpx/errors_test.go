package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapError(t *testing.T) {
	type tc struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}
	sentinels := []struct {
		err    error
		status int
		code   string
	}{
		{ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{ErrValidation, http.StatusBadRequest, "VALIDATION"},
		{ErrConflict, http.StatusConflict, "CONFLICT"},
		{ErrUnprocessable, http.StatusUnprocessableEntity, "UNPROCESSABLE"},
		{ErrInsufficientFunds, http.StatusUnprocessableEntity, "INSUFFICIENT_FUNDS"},
	}

	var tests []tc
	for _, s := range sentinels {
		tests = append(tests,
			tc{s.code + "/direct", s.err, s.status, s.code},
			tc{s.code + "/wrapped", fmt.Errorf("loan 42: %w", s.err), s.status, s.code},
			tc{s.code + "/wrapped twice", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", s.err)), s.status, s.code},
		)
	}
	boom := errors.New("boom")
	tests = append(tests,
		tc{"unknown", boom, http.StatusInternalServerError, "INTERNAL"},
		tc{"unknown wrapped", fmt.Errorf("ctx: %w", boom), http.StatusInternalServerError, "INTERNAL"},
		tc{"stdlib error", context.DeadlineExceeded, http.StatusInternalServerError, "INTERNAL"},
		tc{"nil", nil, http.StatusInternalServerError, "INTERNAL"},
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, code := MapError(tt.err)
			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantCode, code)
		})
	}
}
