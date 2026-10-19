package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type auditValidator struct{}

func (auditValidator) Valid(context.Context, string) (bool, error)    { return true, nil }
func (auditValidator) UserID(context.Context, string) (string, error) { return "user-001", nil }

func TestDailyAPILogs(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewAPILogger(dir)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 10, 19, 23, 59, 59, 0, time.FixedZone("CST", 8*3600))
	logger.now = func() time.Time { return clock }
	s := &GracefulServer{APILogger: logger, TokenValidator: auditValidator{}}
	body := `{"text":"你好\n第二行"}`
	handler := s.logAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/fail" {
			jsonError(w, 401, "无效的Token")
			return
		}
		data := make([]byte, len(body))
		n, _ := r.Body.Read(data)
		if string(data[:n]) != body {
			t.Error("request body changed")
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Write([]byte("private-audio"))
	}))
	for _, path := range []string{"/api/ok", "/api/fail?token=secret"} {
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.Header.Set("Token", "secret")
		handler.ServeHTTP(httptest.NewRecorder(), r)
	}
	first := filepath.Join(dir, "api-2026-10-19.jsonl")
	data, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), "private-audio") {
		t.Fatal("credentials or audio leaked")
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines=%d", len(lines))
	}
	for i, line := range lines {
		var e apiLogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatal(err)
		}
		if e.User != "user-001" || e.RequestBody != body || e.Time != "2026-10-19T23:59:59+08:00" {
			t.Fatalf("incorrect log: %+v", e)
		}
		if i == 0 && !e.Success {
			t.Fatal("success marked failed")
		}
		if i == 1 && (e.Success || e.ErrorCode != 401 || !strings.Contains(e.Error, "无效")) {
			t.Fatalf("missing failure: %+v", e)
		}
	}
	clock = clock.Add(time.Second)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/ok", strings.NewReader(body)))
	if _, err := os.Stat(filepath.Join(dir, "api-2026-10-20.jsonl")); err != nil {
		t.Fatal("no daily rollover:", err)
	}
	// Reclaiming a file must not leave the logger writing to an unlinked inode.
	os.Remove(first)
	clock = clock.Add(-time.Second)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/ok", strings.NewReader(body)))
	if _, err := os.Stat(first); err != nil {
		t.Fatal("did not recreate collected file")
	}
}

func TestConcurrentAndTimedOutLogs(t *testing.T) {
	dir := t.TempDir()
	logger, err := NewAPILogger(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	logger.now = func() time.Time { return now }
	s := &GracefulServer{APILogger: logger}
	var wg sync.WaitGroup
	handler := s.logAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/test", strings.NewReader("test")))
		}()
	}
	wg.Wait()
	finished := make(chan struct{})
	timed := s.logAPI(http.TimeoutHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
		w.Write([]byte("late"))
		close(finished)
	}), time.Millisecond, "timeout"))
	timed.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/test", strings.NewReader("body")))
	<-finished
	data, err := os.ReadFile(logger.path(now))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 31 {
		t.Fatalf("lines=%d", len(lines))
	}
	for _, line := range lines {
		var e apiLogEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatal(err)
		}
	}
	var last apiLogEntry
	json.Unmarshal([]byte(lines[30]), &last)
	if last.Status != 503 || last.Success || last.RequestBody != "body" {
		t.Fatalf("timeout: %+v", last)
	}
}
