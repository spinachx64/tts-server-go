package server

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEdgeJSONValidation(t *testing.T) {
	s := &GracefulServer{Token: "secret"}
	for _, tc := range []struct {
		name, method, contentType, token, body string
		status                                 int
	}{
		{"missing token", "POST", "application/json", "", `{"text":"你好"}`, 401},
		{"wrong token", "POST", "application/json", "wrong", `{"text":"你好"}`, 401},
		{"method", "GET", "application/json", "secret", "", 405},
		{"ssml", "POST", "text/plain", "secret", "<speak/>", 415},
		{"invalid JSON", "POST", "application/json", "secret", "{", 400},
		{"empty text", "POST", "application/json", "secret", "{}", 400},
		{"unknown field", "POST", "application/json", "secret", `{"text":"hi","typo":true}`, 400},
		{"multiple objects", "POST", "application/json", "secret", `{"help":true}{}`, 400},
		{"invalid pitch", "POST", "application/json", "secret", `{"text":"hi","pitch":"0%"}`, 400},
		{"help", "POST", "application/json", "secret", `{"help":true}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/api/ra", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", tc.contentType)
			r.Header.Set("Token", tc.token)
			w := httptest.NewRecorder()
			s.edgeAPIHandler(w, r)
			if w.Code != tc.status {
				t.Fatalf("got %d: %s", w.Code, w.Body.String())
			}
			if !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
				t.Fatal("response is not JSON")
			}
		})
	}
}
func TestEdgeLegadoJSON(t *testing.T) {
	body, err := genLegadoJson("http://localhost:1233/api/ra", "name", "zh-CN-XiaoxiaoNeural", "", "", "", "", "audio-24khz-48kbitrate-mono-mp3", "secret", "5000")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{"JSON.stringify", "response_format", "application/json", "secret"} {
		if !strings.Contains(string(body), part) {
			t.Fatalf("missing %s: %s", part, body)
		}
	}
}
