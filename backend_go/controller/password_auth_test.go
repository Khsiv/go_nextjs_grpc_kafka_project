package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateCredentials(t *testing.T) {
	cases := []struct {
		name     string
		email    string
		password string
		want     string
	}{
		{"normalizes email", "Ada@Example.com", "password1", "ada@example.com"},
		{"trims email", "  ada@example.com  ", "password1", "ada@example.com"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validateCredentials(tc.email, tc.password)
			if err != nil {
				t.Fatalf("validateCredentials: %v", err)
			}
			if got != tc.want {
				t.Errorf("email = %q, want %q", got, tc.want)
			}
		})
	}

	rejected := []struct {
		name     string
		email    string
		password string
	}{
		{"missing email", "", "password1"},
		{"not an email", "not-an-email", "password1"},
		{"short password", "ada@example.com", "short"},
		{"password over bcrypt limit", "ada@example.com", strings.Repeat("x", maxPasswordLength+1)},
	}

	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateCredentials(tc.email, tc.password); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestAuthHandlersRejectBadInputWithoutDatabase(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		body    string
	}{
		{"signup bad json", handleSignup(nil), `{`},
		{"signup short password", handleSignup(nil), `{"email":"ada@example.com","password":"short"}`},
		{"login bad json", handleLogin(nil), `{`},
		{"login invalid email", handleLogin(nil), `{"email":"nope","password":"password1"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/go/auth/login", strings.NewReader(tc.body))
			recorder := httptest.NewRecorder()

			tc.handler.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d, body %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
		})
	}
}
