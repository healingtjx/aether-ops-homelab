package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthenticateRedirectsPageRequestToLoginWhenSessionMissing(t *testing.T) {
	handler := Authenticate(nil, "/bs")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/bs" {
		t.Fatalf("Location = %q, want /bs", got)
	}
}

func TestAuthenticateReturnsHXRedirectForHtmxRequestWhenSessionMissing(t *testing.T) {
	handler := Authenticate(nil, "/bs")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/projects", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := rec.Header().Get("HX-Redirect"); got != "/bs" {
		t.Fatalf("HX-Redirect = %q, want /bs", got)
	}
}

func TestAuthenticateReturnsUnauthorizedForFetchRequestWhenSessionMissing(t *testing.T) {
	handler := Authenticate(nil, "/bs")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodPost, "/ci-configs/1/run/prepare", nil)
	req.Header.Set("X-Requested-With", "fetch")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := rec.Header().Get("X-Login-Path"); got != "/bs" {
		t.Fatalf("X-Login-Path = %q, want /bs", got)
	}
}

func TestAuthenticateReturnsAuthExpiredEventForEventSourceWhenSessionMissing(t *testing.T) {
	handler := Authenticate(nil, "/bs")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/applications/1/pull/stream", nil)
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "event: auth-expired\ndata: /bs\n\n" {
		t.Fatalf("body = %q", got)
	}
}
