package httpserver

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReadiness(t *testing.T) {
	for _, tc := range []struct {
		method string
		failed bool
		status int
	}{{"GET", false, 200}, {"GET", true, 503}, {"HEAD", false, 200}, {"POST", false, 405}} {
		handler := Readiness(func(ctx context.Context) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Error("missing ping timeout")
			}
			if tc.failed {
				return errors.New("private database details")
			}
			return nil
		})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(tc.method, "/healthz", nil))
		if response.Code != tc.status {
			t.Fatal(response.Code, tc)
		}
		if strings.Contains(response.Body.String(), "private") {
			t.Fatal("database error leaked")
		}
		if tc.method == "HEAD" && response.Body.Len() != 0 {
			t.Fatal("HEAD body")
		}
	}
}
