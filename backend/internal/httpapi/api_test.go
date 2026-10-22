package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestHealthRouteIncludesSecurityHeaders(t *testing.T) {
	t.Setenv("JWT_SECRET", "local-test-secret-with-at-least-32-bytes")
	api, err := New(nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	recorder := httptest.NewRecorder()
	api.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/health/live", nil))

	if recorder.Code != http.StatusOK || recorder.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("unexpected health response: status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if got := recorder.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", got)
	}
}

func TestSelfServiceRoutesAreRegisteredAndAuthenticated(t *testing.T) {
	t.Setenv("JWT_SECRET", "local-test-secret-with-at-least-32-bytes")
	api, err := New(nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	for _, route := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/employee-requests/me"},
		{http.MethodGet, "/api/v1/employee-requests/approvals"},
		{http.MethodPost, "/api/v1/employee-requests"},
		{http.MethodPost, "/api/v1/leave/requests/00000000-0000-0000-0000-000000000001/superadmin-decision"},
		{http.MethodGet, "/api/v1/attendance/today"},
	} {
		recorder := httptest.NewRecorder()
		api.Handler().ServeHTTP(recorder, httptest.NewRequest(route.method, route.path, nil))
		if recorder.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status = %d, want %d", route.method, route.path, recorder.Code, http.StatusUnauthorized)
		}
	}
}

func TestLoginLimiter(t *testing.T) {
	limiter := &loginLimiter{
		attempts: make(map[string]attemptWindow),
		limit:    2,
		window:   time.Minute,
	}
	if !limiter.allow("192.0.2.1") || !limiter.allow("192.0.2.1") {
		t.Fatal("login limiter rejected an attempt below its limit")
	}
	if limiter.allow("192.0.2.1") {
		t.Fatal("login limiter allowed an attempt above its limit")
	}
	if !limiter.allow("192.0.2.2") {
		t.Fatal("login limiter incorrectly shared limits between clients")
	}
}

func TestSameOriginRejectsCrossSiteMutation(t *testing.T) {
	called := false
	handler := (&API{}).sameOrigin(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	request := httptest.NewRequest(http.MethodPost, "https://hris.example.test/api/v1/auth/logout", nil)
	request.Host = "hris.example.test"
	request.Header.Set("Origin", "https://attacker.example")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
	if called {
		t.Fatal("cross-origin handler was invoked")
	}
}

func TestSameOriginAllowsSameOriginMutation(t *testing.T) {
	called := false
	handler := (&API{}).sameOrigin(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodPost, "https://hris.example.test/api/v1/auth/logout", nil)
	request.Host = "hris.example.test"
	request.Header.Set("Origin", "https://hris.example.test")
	request.Header.Set("Sec-Fetch-Site", "same-origin")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent || !called {
		t.Fatalf("same-origin request status = %d, handler called = %v", recorder.Code, called)
	}
}

func TestSameOriginRejectsCrossSiteFetchMetadata(t *testing.T) {
	handler := (&API{}).sameOrigin(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("cross-site handler was invoked")
	}))
	request := httptest.NewRequest(http.MethodPut, "https://hris.example.test/api/v1/auth/password", nil)
	request.Host = "hris.example.test"
	request.Header.Set("Sec-Fetch-Site", "cross-site")

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusForbidden)
	}
}

func TestNewRejectsMissingJWTSecret(t *testing.T) {
	oldValue, hadValue := os.LookupEnv("JWT_SECRET")
	if err := os.Unsetenv("JWT_SECRET"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadValue {
			_ = os.Setenv("JWT_SECRET", oldValue)
		} else {
			_ = os.Unsetenv("JWT_SECRET")
		}
	})
	if _, err := New(nil); err == nil {
		t.Fatal("New() accepted a missing JWT secret")
	}
}
