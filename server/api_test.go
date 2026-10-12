package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestShutdown(t *testing.T) {
	s := &GracefulServer{shutdownLoad: make(chan struct{})}
	s.HandleFunc()
	srv := httptest.NewServer(s.serveMux)
	defer srv.Close()
	s.Server = srv.Config
	done := s.shutdownLoad
	resp, err := srv.Client().Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("首页状态码: %d", resp.StatusCode)
	}
	if err := s.Shutdown(time.Second); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	default:
		t.Fatal("关闭后未通知等待方")
	}
}
