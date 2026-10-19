package server

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

type testTokenValidator struct {
	valid bool
	err   error
}

func (v testTokenValidator) Valid(context.Context, string) (bool, error) { return v.valid, v.err }

func TestSQLiteAuthenticationAcrossSynthesisRoutes(t *testing.T) {
	for _, path := range []string{"/api/ra", "/api/azure", "/api/creation"} {
		for _, tc := range []struct {
			validator testTokenValidator
			status    int
		}{
			{testTokenValidator{}, 401},
			{testTokenValidator{err: errors.New("database unavailable")}, 503},
		} {
			s := &GracefulServer{Token: "legacy", TokenValidator: tc.validator}
			s.HandleFunc()
			r := httptest.NewRequest("POST", path, strings.NewReader("{}"))
			r.Header.Set("Token", "legacy")
			w := httptest.NewRecorder()
			s.serveMux.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("%s: got %d want %d", path, w.Code, tc.status)
			}
		}
	}
}

func TestValidSQLiteTokenAcceptedByEdge(t *testing.T) {
	s := &GracefulServer{TokenValidator: testTokenValidator{valid: true}}
	r := httptest.NewRequest("POST", "/api/ra", strings.NewReader(`{"help":true}`))
	r.Header.Set("Token", "issued-by-external-app")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.edgeAPIHandler(w, r)
	if w.Code != 200 {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
}
