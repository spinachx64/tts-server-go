package edge

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// Output accepts a CLI-style download filename or a boolean switch.
type Output struct {
	Enabled bool
	Name    string
}

func (o *Output) UnmarshalJSON(b []byte) error {
	if err := json.Unmarshal(b, &o.Enabled); err == nil {
		return nil
	}
	if err := json.Unmarshal(b, &o.Name); err != nil {
		return errors.New("write_subtitles 必须是布尔值或文件名")
	}
	o.Enabled = o.Name != ""
	return nil
}

type TextFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}
type Options struct {
	Text           string    `json:"text"`
	File           *TextFile `json:"file,omitempty"`
	Voice          string    `json:"voice"`
	Rate           string    `json:"rate"`
	Volume         string    `json:"volume"`
	Pitch          string    `json:"pitch"`
	Proxy          string    `json:"proxy"`
	Format         string    `json:"format"`
	WriteMedia     string    `json:"write_media"`
	WriteSubtitles Output    `json:"write_subtitles"`
	ResponseFormat string    `json:"response_format"`
	ListVoices     bool      `json:"list_voices"`
	Version        bool      `json:"version"`
	Help           bool      `json:"help"`
}

var percentPattern = regexp.MustCompile(`^[+-]?[0-9]+%$`)
var pitchPattern = regexp.MustCompile(`^[+-]?[0-9]+Hz$`)
var voicePattern = regexp.MustCompile(`^[a-z]{2,3}-[A-Z]{2}(-[a-zA-Z0-9]+)+Neural$`)

func (o *Options) Validate() error {
	if o.Proxy != "" {
		if _, err := proxyFunction(o.Proxy); err != nil {
			return err
		}
	}
	if o.ListVoices || o.Version || o.Help {
		count := 0
		for _, v := range []bool{o.ListVoices, o.Version, o.Help} {
			if v {
				count++
			}
		}
		if count != 1 || o.Text != "" || o.File != nil {
			return errors.New("list_voices、version、help 与合成输入互斥")
		}
		return nil
	}
	if o.File != nil {
		if o.Text != "" {
			return errors.New("text 和 file 只能选一个")
		}
		o.Text = o.File.Content
	}
	if strings.TrimSpace(o.Text) == "" {
		return errors.New("text 或 file.content 不能为空")
	}
	if !utf8.ValidString(o.Text) {
		return errors.New("文本必须是 UTF-8")
	}
	if o.Voice == "" {
		o.Voice = "en-US-EmmaMultilingualNeural"
	}
	if !voicePattern.MatchString(o.Voice) {
		return errors.New("voice 必须是完整 Neural 音色 ID")
	}
	if o.Rate == "" {
		o.Rate = "+0%"
	}
	if o.Volume == "" {
		o.Volume = "+0%"
	}
	if o.Pitch == "" {
		o.Pitch = "+0Hz"
	}
	if !percentPattern.MatchString(o.Rate) || !percentPattern.MatchString(o.Volume) || !pitchPattern.MatchString(o.Pitch) {
		return errors.New("rate/volume 使用整数百分比（如 -10%），pitch 使用整数 Hz（如 +5Hz）")
	}
	for _, field := range []*string{&o.Rate, &o.Volume, &o.Pitch} {
		if (*field)[0] != '+' && (*field)[0] != '-' {
			*field = "+" + *field
		}
	}
	if o.Format == "" {
		o.Format = "audio-24khz-48kbitrate-mono-mp3"
	}
	if o.Format == "webm-24khz-16bit-mono-opus" && len(textChunks(o.Text)) > 1 {
		return errors.New("长文本分段请使用 MP3 格式")
	}
	switch o.Format {
	case "audio-24khz-48kbitrate-mono-mp3", "audio-24khz-96kbitrate-mono-mp3", "webm-24khz-16bit-mono-opus":
	default:
		return errors.New("不支持的 format")
	}
	if o.WriteSubtitles.Enabled && o.Format != "audio-24khz-48kbitrate-mono-mp3" {
		return errors.New("字幕需要 audio-24khz-48kbitrate-mono-mp3 格式")
	}
	if o.ResponseFormat == "" {
		o.ResponseFormat = "json"
	}
	if o.ResponseFormat != "json" && o.ResponseFormat != "audio" {
		return errors.New("response_format 必须是 json 或 audio")
	}
	if o.ResponseFormat == "audio" && o.WriteSubtitles.Enabled {
		return errors.New("同时返回音频和字幕请使用 response_format=json")
	}
	if o.WriteMedia == "" {
		if strings.HasPrefix(o.Format, "webm") {
			o.WriteMedia = "audio.webm"
		} else {
			o.WriteMedia = "audio.mp3"
		}
	}
	for _, name := range []string{o.WriteMedia, o.WriteSubtitles.Name} {
		if name != "" && (filepath.Base(name) != name || strings.ContainsAny(name, "\\\r\n\"")) {
			return errors.New("输出文件名不能包含目录、引号或换行")
		}
	}
	return nil
}
func escaped(s string) string { var b bytes.Buffer; xml.EscapeText(&b, []byte(s)); return b.String() }
func textChunks(text string) []string {
	var chunks []string
	var b strings.Builder
	for _, r := range text {
		if (r < 32 && r != '\n' && r != '\r' && r != '\t') || r == 0xFFFE || r == 0xFFFF {
			r = ' '
		}
		part := escaped(string(r))
		if b.Len()+len(part) > 3000 {
			chunks = append(chunks, b.String())
			b.Reset()
		}
		b.WriteString(part)
	}
	if b.Len() > 0 {
		chunks = append(chunks, b.String())
	}
	return chunks
}
func Synthesize(ctx context.Context, o Options) (Result, error) {
	var all Result
	var offset int64
	for _, text := range textChunks(o.Text) {
		conn, err := dial(ctx, o.Proxy, 30*time.Second)
		if err != nil {
			return all, err
		}
		ssml := `<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="en-US"><voice name="` + escaped(o.Voice) + `"><prosody rate="` + o.Rate + `" volume="` + o.Volume + `" pitch="` + o.Pitch + `">` + text + `</prosody></voice></speak>`
		result, err := readTurn(ctx, conn, ssml, o.Format, o.WriteSubtitles.Enabled, 0)
		conn.Close()
		if err != nil {
			return all, err
		}
		all.Audio = append(all.Audio, result.Audio...)
		for _, b := range result.Boundaries {
			b.Offset += offset
			all.Boundaries = append(all.Boundaries, b)
		}
		// Same 48 kbps CBR compensation used by edge-tts across chunks (100 ns ticks).
		offset += int64(len(result.Audio)) * 8 * 10000000 / 48000
	}
	return all, nil
}
func SRT(boundaries []Boundary) string {
	var b strings.Builder
	for i, cue := range boundaries {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, srtTime(cue.Offset), srtTime(cue.Offset+cue.Duration), strings.ReplaceAll(strings.ReplaceAll(cue.Text, "\r", " "), "\n", " "))
	}
	return b.String()
}
func srtTime(ticks int64) string {
	ms := ticks / 10000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", ms/3600000, ms/60000%60, ms/1000%60, ms%1000)
}
func Voices(ctx context.Context, proxy string) (json.RawMessage, error) {
	proxyFunc, err := proxyFunction(proxy)
	if err != nil {
		return nil, err
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = proxyFunc
	defer tr.CloseIdleConnections()
	client := http.Client{Transport: tr, Timeout: 30 * time.Second}
	now := time.Now()
	for attempt := 0; attempt < 2; attempt++ {
		endpoint := "https://speech.platform.bing.com/consumer/speech/synthesize/readaloud/voices/list?trustedclienttoken=" + trustedClientToken + "&Sec-MS-GEC=" + generateSecMSGEC(now) + "&Sec-MS-GEC-Version=" + secMSGECVersion
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header = requestHeaders()
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if resp.StatusCode == 403 && attempt == 0 {
			now, err = http.ParseTime(resp.Header.Get("Date"))
			if err == nil {
				continue
			}
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("声音列表: %s", resp.Status)
		}
		if readErr != nil {
			return nil, readErr
		}
		var voices []json.RawMessage
		if err = json.Unmarshal(data, &voices); err != nil {
			return nil, err
		}
		return data, nil
	}
	return nil, errors.New("声音列表请求失败")
}
