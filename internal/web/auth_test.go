package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTokenAuth(t *testing.T) {
	server, _ := testServer(t)
	server.token = "0123456789abcdef"
	handler := server.Handler()

	rec := request(t, handler, http.MethodGet, "/api/status", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token status = %d, want 401", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("X-FFBatch-Token", "0123456789abcdef")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("header token status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer 0123456789abcdef")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("bearer token status = %d, want 200", rec.Code)
	}

	rec = request(t, handler, http.MethodGet, "/api/status?token=0123456789abcdef", nil, "")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("query token status = %d, want 303", rec.Code)
	}
	if location := rec.Header().Get("Location"); location != "/" {
		t.Fatalf("redirect location = %q, want /", location)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != authCookieName || !cookies[0].HttpOnly {
		t.Fatalf("unexpected cookies: %+v", cookies)
	}
	if strings.Contains(rec.Header().Get("Location"), "token") {
		t.Fatal("redirect leaked the token")
	}

	req = httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.AddCookie(cookies[0])
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cookie status = %d, want 200", rec.Code)
	}

	rec = request(t, handler, http.MethodGet, "/api/status?token=wrong", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token status = %d, want 401", rec.Code)
	}
}

func TestTokenRequiredForHTML(t *testing.T) {
	server, _ := testServer(t)
	server.token = "0123456789abcdef"
	rec := request(t, server.Handler(), http.MethodGet, "/", nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("index status = %d, want 401", rec.Code)
	}
}

func TestTokenizedURL(t *testing.T) {
	got := TokenizedURL("http://127.0.0.1:8737/", "abc def")
	if !strings.Contains(got, "token=abc+def") {
		t.Fatalf("tokenized URL = %q", got)
	}
	if plain := TokenizedURL("http://127.0.0.1:8737/", ""); plain != "http://127.0.0.1:8737/" {
		t.Fatalf("plain URL = %q", plain)
	}
}
