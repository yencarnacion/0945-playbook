package data

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestMassiveStreamAgainstMockWebSocketServer(t *testing.T) {
	up := websocket.Upgrader{}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("local sockets unavailable: %v", err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_, auth, err := c.ReadMessage()
		if err != nil {
			return
		}
		if !strings.Contains(string(auth), `"action":"auth"`) {
			t.Errorf("first message=%s", auth)
			return
		}
		_ = c.WriteMessage(websocket.TextMessage, []byte(`[{"ev":"status","status":"auth_success","message":"authenticated"}]`))
		_, sub, err := c.ReadMessage()
		if err != nil {
			return
		}
		if !strings.Contains(string(sub), "A.AAPL") {
			t.Errorf("subscription=%s", sub)
			return
		}
		now := time.Now().UnixMilli()
		payload := `[{"ev":"A","sym":"AAPL","v":100,"o":10,"h":10.2,"l":9.9,"c":10.1,"vw":10.05,"s":` + itoa(now-1000) + `,"e":` + itoa(now) + `}]`
		_ = c.WriteMessage(websocket.TextMessage, []byte(payload))
	}))
	server.Listener = ln
	server.Start()
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http")
	s, err := NewMassiveStream("test-key", url, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Subscribe([]string{"AAPL"}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	select {
	case ev := <-s.Events():
		if ev.Symbol != "AAPL" || ev.Close != 10.1 || ev.Received.IsZero() {
			t.Fatalf("event=%+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for aggregate")
	}
}
func itoa(v int64) string {
	const digits = "0123456789"
	if v == 0 {
		return "0"
	}
	var b [24]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = digits[v%10]
		v /= 10
	}
	return string(b[i:])
}
