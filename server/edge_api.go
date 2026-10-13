package server

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/jing332/tts-server-go/tts/edge"
)

func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func jsonError(w http.ResponseWriter, status int, err string) {
	jsonResponse(w, status, map[string]string{"error": err})
}

func (s *GracefulServer) edgeAPIHandler(w http.ResponseWriter, r *http.Request) {
	if s.Token != "" && subtle.ConstantTimeCompare([]byte(s.Token), []byte(r.Header.Get("Token"))) != 1 {
		jsonError(w, 401, "无效的Token")
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		jsonError(w, 405, "请使用 POST")
		return
	}
	contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || contentType != "application/json" {
		jsonError(w, 415, "Content-Type 必须是 application/json")
		return
	}
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	decoder.DisallowUnknownFields()
	var options edge.Options
	if err := decoder.Decode(&options); err != nil {
		jsonError(w, 400, "无效 JSON: "+err.Error())
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		jsonError(w, 400, "请求只能包含一个 JSON 对象")
		return
	}
	if err := options.Validate(); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	if options.Help || options.Version {
		jsonResponse(w, 200, map[string]any{"backend": "Go native Edge TTS", "gec_version": "1-143.0.3650.75", "default_voice": "en-US-EmmaMultilingualNeural", "parameters": []string{"text", "file", "voice", "rate", "volume", "pitch", "proxy", "format", "write_media", "write_subtitles", "response_format", "list_voices", "version", "help"}})
		return
	}
	if options.ListVoices {
		voices, err := edge.Voices(r.Context(), options.Proxy)
		if err != nil {
			jsonError(w, 502, err.Error())
			return
		}
		jsonResponse(w, 200, map[string]any{"voices": voices})
		return
	}
	result, err := edge.Synthesize(r.Context(), options)
	if err != nil {
		if r.Context().Err() != nil {
			jsonError(w, 504, "请求超时或取消")
		} else {
			jsonError(w, 502, err.Error())
		}
		return
	}
	contentType = "audio/mpeg"
	if strings.HasPrefix(options.Format, "webm") {
		contentType = "audio/webm"
	}
	if options.ResponseFormat == "audio" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": options.WriteMedia}))
		_ = writeAudioData(w, result.Audio, options.Format)
		return
	}
	response := map[string]any{"audio": result.Audio, "encoding": "base64", "content_type": contentType, "format": options.Format, "media_name": options.WriteMedia}
	if options.WriteSubtitles.Enabled {
		name := options.WriteSubtitles.Name
		if name == "" {
			name = "audio.srt"
		}
		response["subtitles"] = edge.SRT(result.Boundaries)
		response["subtitles_name"] = name
		if len(result.Boundaries) == 0 {
			jsonError(w, 502, "微软接口未返回字幕元数据")
			return
		}
	}
	jsonResponse(w, 200, response)
}
