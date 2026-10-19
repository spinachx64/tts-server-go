package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// APILogger writes JSON Lines; opening on each append allows old files to be
// collected without keeping deleted files open. It never records auth headers.
type APILogger struct {
	dir string
	mu  sync.Mutex
	now func() time.Time
}

type apiLogEntry struct {
	Time          string `json:"time"`
	CompletedAt   string `json:"completed_at"`
	User          string `json:"username"`
	Method        string `json:"method"`
	Path          string `json:"path"`
	Query         string `json:"query,omitempty"`
	RequestBody   string `json:"request_body"`
	Success       bool   `json:"success"`
	Status        int    `json:"status_code"`
	ErrorCode     int    `json:"error_code,omitempty"`
	Error         string `json:"error,omitempty"`
	ResponseBytes int    `json:"response_bytes"`
	DurationMS    int64  `json:"duration_ms"`
}

func NewAPILogger(dir string) (*APILogger, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	l := &APILogger{dir: dir, now: time.Now}
	// Check write permissions at startup, before accepting traffic.
	f, err := os.OpenFile(l.path(l.now()), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return l, f.Close()
}
func (l *APILogger) path(t time.Time) string {
	return filepath.Join(l.dir, "api-"+t.Format("2006-01-02")+".jsonl")
}
func (l *APILogger) append(t time.Time, entry apiLogEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path(t), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

type replayBody struct {
	io.Reader
	io.Closer
}

type loggedResponse struct {
	http.ResponseWriter
	status, size int
	failure      bytes.Buffer
}

func (w *loggedResponse) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *loggedResponse) WriteHeader(status int) {
	if status >= 100 && status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *loggedResponse) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.size += n
	if w.status >= 400 {
		keep := 4096 - w.failure.Len()
		if keep > n {
			keep = n
		}
		w.failure.Write(p[:keep])
	}
	return n, err
}

func (s *GracefulServer) logAPI(next http.Handler) http.Handler {
	if s.APILogger == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		start := s.APILogger.now()
		user := "anonymous"
		token := r.Header.Get("Token")
		if lookup, ok := s.TokenValidator.(interface {
			UserID(context.Context, string) (string, error)
		}); ok {
			user = "unknown"
			name, err := lookup.UserID(r.Context(), token)
			if err == nil && name != "" {
				user = name
			}
		} else if s.Token != "" {
			user = "unknown"
			if token == s.Token {
				user = "shared"
			}
		}
		if r.URL.Path == "/api/tokens" {
			user = "unknown"
			if s.validAdmin(r) {
				user = "admin"
			}
		}
		originalBody := r.Body
		if originalBody == nil {
			originalBody = http.NoBody
		}
		// Capture even requests rejected before authentication/body parsing. Replay
		// exactly the original bytes to the handler without changing its body limits.
		uploaded, readErr := io.ReadAll(originalBody)
		r = r.Clone(r.Context())
		r.Body = replayBody{Reader: bytes.NewReader(uploaded), Closer: originalBody}
		response := &loggedResponse{ResponseWriter: w}
		defer func() {
			panicValue := recover()
			status := response.status
			if status == 0 {
				status = 200
			}
			if panicValue != nil {
				status = 500
			}
			query := r.URL.Query()
			for key := range query {
				if strings.EqualFold(key, "token") {
					query.Set(key, "[REDACTED]")
				}
			}
			// Parse and encode rather than logging the raw query containing credentials.
			safeQuery := url.Values(query).Encode()
			completed := s.APILogger.now()
			entry := apiLogEntry{Time: start.Format(time.RFC3339Nano), CompletedAt: completed.Format(time.RFC3339Nano), User: user, Method: r.Method, Path: r.URL.Path, Query: safeQuery, RequestBody: string(uploaded), Success: status >= 200 && status < 400, Status: status, ResponseBytes: response.size, DurationMS: completed.Sub(start).Milliseconds()}
			if !entry.Success {
				entry.ErrorCode = status
				entry.Error = response.failure.String()
			}
			if panicValue != nil {
				entry.Error = "handler panic"
			}
			if err := s.APILogger.append(start, entry); err != nil {
				log.WithError(err).Error("写入API调用日志失败")
			}
			if panicValue != nil {
				panic(panicValue)
			}
		}()
		if readErr != nil {
			jsonError(response, http.StatusBadRequest, "读取请求内容失败")
			return
		}
		next.ServeHTTP(response, r)
	})
}
