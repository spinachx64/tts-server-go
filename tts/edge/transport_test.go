package edge

import (
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestReadTurnFrames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for i := 0; i < 2; i++ {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
		headers := []byte("Content-Type:audio/mpeg\r\nPath:audio\r\n")
		frame := make([]byte, 2)
		binary.BigEndian.PutUint16(frame, uint16(len(headers)))
		frame = append(frame, headers...)
		frame = append(frame, []byte("ID3testaudio")...)
		conn.WriteMessage(websocket.BinaryMessage, frame)
		conn.WriteMessage(websocket.TextMessage, []byte("Path:audio.metadata\r\n\r\n"+`{"Metadata":[{"Type":"WordBoundary","Data":{"Offset":500000,"Duration":1000000,"text":{"Text":"你好"}}}]}`))
		conn.WriteMessage(websocket.TextMessage, []byte("Path:turn.end\r\n\r\n"))
	}))
	defer srv.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := readTurn(ctx, conn, "<speak/>", "audio-24khz-48kbitrate-mono-mp3", true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Audio) != "ID3testaudio" || len(result.Boundaries) != 1 || result.Boundaries[0].Text != "你好" {
		t.Fatalf("bad result %+v", result)
	}
}
func TestReadTurnCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = readTurn(ctx, conn, "<speak/>", "audio-24khz-48kbitrate-mono-mp3", false, 0)
	if err == nil {
		t.Fatal("canceled request returned success")
	}
	if ctx.Err() != context.DeadlineExceeded {
		t.Fatalf("wrong cancellation: %v", ctx.Err())
	}
}
