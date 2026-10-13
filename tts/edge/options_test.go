package edge

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestOptions(t *testing.T) {
	for _, body := range []string{`{"text":"hi","voice":"bad"}`, `{"text":"hi","rate":"fast"}`, `{"text":"hi","proxy":"file:///tmp/test"}`, `{"text":"hi","file":{"content":"hi"}}`, `{"text":"hi","write_media":"../audio.mp3"}`, `{"text":"hi","write_subtitles":true,"response_format":"audio"}`} {
		var o Options
		if err := json.Unmarshal([]byte(body), &o); err != nil {
			t.Fatal(err)
		}
		if o.Validate() == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	var o Options
	if err := json.Unmarshal([]byte(`{"file":{"name":"text.txt","content":"hello"},"rate":"10%","pitch":"5Hz","write_subtitles":"hello.srt"}`), &o); err != nil {
		t.Fatal(err)
	}
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	if o.Text != "hello" || o.Rate != "+10%" || o.Pitch != "+5Hz" || !o.WriteSubtitles.Enabled || o.WriteSubtitles.Name != "hello.srt" {
		t.Fatalf("bad defaults: %+v", o)
	}
}
func TestTextChunks(t *testing.T) {
	chunks := textChunks(strings.Repeat("中文<&", 1500) + "\x01")
	if len(chunks) < 2 {
		t.Fatal("not split")
	}
	for _, chunk := range chunks {
		if len(chunk) > 3000 || !utf8.ValidString(chunk) || strings.Contains(chunk, "\x01") {
			t.Fatal("invalid chunk")
		}
	}
	if !strings.Contains(strings.Join(chunks, ""), "&lt;&amp;") {
		t.Fatal("XML not escaped")
	}
}
func TestSRT(t *testing.T) {
	got := SRT([]Boundary{{Offset: 12340000, Duration: 5000000, Text: "中文"}})
	if got != "1\n00:00:01,234 --> 00:00:01,734\n中文\n\n" {
		t.Fatal(got)
	}
}
