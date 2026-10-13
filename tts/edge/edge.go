package edge

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	tsg "github.com/jing332/tts-server-go"
)

const (
	trustedClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	secMSGECVersion    = "1-143.0.3650.75"
	wssUrl             = "wss://api.msedgeservices.com/tts/cognitiveservices/websocket/v1?Ocp-Apim-Subscription-Key=" + trustedClientToken
	userAgent          = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
)

type TTS struct {
	DnsLookupEnabled          bool // Deprecated: 新接口始终使用 DNS。
	DialTimeout, WriteTimeout time.Duration
	conn                      *websocket.Conn
}

func requestID() string { return strings.ReplaceAll(tsg.GetUUID(), "-", "") }
func requestHeaders() http.Header {
	h := http.Header{}
	h.Set("User-Agent", userAgent)
	h.Set("Accept-Language", "en-US,en;q=0.9")
	h.Set("Pragma", "no-cache")
	h.Set("Cache-Control", "no-cache")
	h.Set("Cookie", "muid="+strings.ToUpper(requestID())+";")
	return h
}

func proxyFunction(proxy string) (func(*http.Request) (*url.URL, error), error) {
	if proxy == "" {
		return http.ProxyFromEnvironment, nil
	}
	u, err := url.Parse(proxy)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("proxy 必须是 http 或 https URL")
	}
	return http.ProxyURL(u), nil
}

func dial(ctx context.Context, proxy string, timeout time.Duration) (*websocket.Conn, error) {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	proxyFunc, err := proxyFunction(proxy)
	if err != nil {
		return nil, err
	}
	d := websocket.Dialer{EnableCompression: true, Proxy: proxyFunc, HandshakeTimeout: timeout}
	h := requestHeaders()
	h.Set("Origin", "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold")
	now := time.Now()
	id := requestID()
	for attempt := 0; attempt < 2; attempt++ {
		endpoint := wssUrl + "&ConnectionId=" + id + "&Sec-MS-GEC=" + generateSecMSGEC(now) + "&Sec-MS-GEC-Version=" + secMSGECVersion
		conn, resp, err := d.DialContext(ctx, endpoint, h)
		if err == nil {
			return conn, nil
		}
		if resp == nil {
			return nil, err
		}
		if resp.Body != nil {
			resp.Body.Close()
		}
		if resp.StatusCode != http.StatusForbidden || attempt == 1 {
			return nil, fmt.Errorf("%w: %s", err, resp.Status)
		}
		now, err = http.ParseTime(resp.Header.Get("Date"))
		if err != nil {
			return nil, fmt.Errorf("403 响应缺少有效 Date: %w", err)
		}
	}
	return nil, errors.New("连接失败")
}

func (t *TTS) NewConn() (err error) {
	t.conn, err = dial(context.Background(), "", t.DialTimeout)
	return
}
func (t *TTS) CloseConn() {
	if t.conn != nil {
		t.conn.Close()
		t.conn = nil
	}
}
func (t *TTS) GetAudio(ssml, format string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if t.conn == nil {
		var err error
		t.conn, err = dial(ctx, "", t.DialTimeout)
		if err != nil {
			return nil, err
		}
	}
	defer t.CloseConn()
	result, err := readTurn(ctx, t.conn, ssml, format, false, t.WriteTimeout)
	return result.Audio, err
}

type Boundary struct {
	Offset   int64
	Duration int64
	Text     string
}
type Result struct {
	Audio      []byte
	Boundaries []Boundary
}

func readTurn(ctx context.Context, conn *websocket.Conn, ssml, format string, subtitles bool, writeTimeout time.Duration) (Result, error) {
	var result Result
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetReadDeadline(deadline)
	}
	conn.SetReadLimit(8 << 20)
	if writeTimeout == 0 {
		writeTimeout = 5 * time.Second
	}
	config := map[string]any{"context": map[string]any{"synthesis": map[string]any{"audio": map[string]any{"metadataoptions": map[string]string{"sentenceBoundaryEnabled": "false", "wordBoundaryEnabled": fmt.Sprint(subtitles)}, "outputFormat": format}}}}
	body, _ := json.Marshal(config)
	for _, msg := range []string{
		"X-Timestamp:" + edgeTimestamp() + "\r\nContent-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n" + string(body) + "\r\n",
		"X-RequestId:" + requestID() + "\r\nContent-Type:application/ssml+xml\r\nX-Timestamp:" + edgeTimestamp() + "Z\r\nPath:ssml\r\n\r\n" + ssml,
	} {
		conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			return result, err
		}
	}
	for {
		kind, frame, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			return result, err
		}
		var headers, payload []byte
		if kind == websocket.BinaryMessage {
			if len(frame) < 2 {
				return result, errors.New("音频帧缺少长度")
			}
			n := int(binary.BigEndian.Uint16(frame[:2]))
			if n > len(frame)-2 {
				return result, errors.New("无效音频帧长度")
			}
			headers, payload = frame[2:2+n], frame[2+n:]
		} else if kind == websocket.TextMessage {
			headers, payload, _ = bytes.Cut(frame, []byte("\r\n\r\n"))
		} else {
			continue
		}
		path := ""
		for _, line := range strings.Split(string(headers), "\r\n") {
			key, value, ok := strings.Cut(line, ":")
			if ok && strings.EqualFold(strings.TrimSpace(key), "Path") {
				path = strings.TrimSpace(value)
			}
		}
		switch path {
		case "audio":
			result.Audio = append(result.Audio, payload...)
		case "audio.metadata":
			var metadata struct {
				Metadata []struct {
					Type string
					Data struct {
						Offset   int64
						Duration int64
						Text     struct{ Text string }
					}
				}
			}
			if err := json.Unmarshal(payload, &metadata); err != nil {
				return result, fmt.Errorf("字幕元数据: %w", err)
			}
			for _, m := range metadata.Metadata {
				if m.Type == "WordBoundary" {
					result.Boundaries = append(result.Boundaries, Boundary{m.Data.Offset, m.Data.Duration, m.Data.Text.Text})
				}
			}
		case "turn.end":
			if len(result.Audio) == 0 {
				return result, errors.New("微软接口未返回音频")
			}
			return result, nil
		}
	}
}

func generateSecMSGEC(now time.Time) string {
	seconds := now.Unix() + 11644473600
	ticks := (seconds - seconds%300) * 10000000
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d%s", ticks, trustedClientToken)))
	return fmt.Sprintf("%X", digest)
}
func edgeTimestamp() string {
	return time.Now().UTC().Format("Mon Jan 02 2006 15:04:05 GMT+0000 (Coordinated Universal Time)")
}
