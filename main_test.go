package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// ── test helpers ────────────────────────────────────────────────────────────

// testEnv starts a testServer and returns the WS URL, control base URL,
// and a cleanup function.
func testEnv(t *testing.T) (wsURL string, controlURL string) {
	t.Helper()

	logger := zap.Must(zap.NewDevelopment())
	ts := newTestServer(logger)

	wsPort, err := ts.startWebSocket()
	if err != nil {
		t.Fatalf("startWebSocket: %v", err)
	}

	ctlPort, err := ts.startControl()
	if err != nil {
		t.Fatalf("startControl: %v", err)
	}

	t.Cleanup(ts.close)

	wsURL = fmt.Sprintf("ws://127.0.0.1:%d", wsPort)
	controlURL = fmt.Sprintf("http://127.0.0.1:%d", ctlPort)
	return wsURL, controlURL
}

// controlPost sends a POST to the control endpoint and returns the response.
func controlPost(t *testing.T, baseURL, path string) response {
	t.Helper()

	resp, err := http.Post(baseURL+path, "", nil)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	var r response
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, body)
	}
	return r
}

// controlGet sends a GET to the control endpoint and returns the HTTP status.
func controlGet(t *testing.T, baseURL, path string) int {
	t.Helper()

	resp, err := http.Get(baseURL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

// dialWS dials the WebSocket server with an optional query string and
// returns the connection.
func dialWS(t *testing.T, wsURL string, query string) *websocket.Conn {
	t.Helper()

	url := wsURL
	if query != "" {
		url += "?" + query
	}

	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// ── tests ───────────────────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	_, controlURL := testEnv(t)

	status := controlGet(t, controlURL, "/health")
	if status != http.StatusOK {
		t.Fatalf("health: want 200, got %d", status)
	}
}

func TestEcho(t *testing.T) {
	wsURL, _ := testEnv(t)

	conn := dialWS(t, wsURL, "id=echo-1")

	frame := map[string]any{"event": "ping", "payload": "pong"}
	if err := conn.WriteJSON(frame); err != nil {
		t.Fatalf("write: %v", err)
	}

	var got map[string]any
	if err := conn.ReadJSON(&got); err != nil {
		t.Fatalf("read: %v", err)
	}

	if got["event"] != "ping" {
		t.Errorf("event: want %q, got %q", "ping", got["event"])
	}
}

func TestReject(t *testing.T) {
	wsURL, _ := testEnv(t)

	_, _, err := websocket.DefaultDialer.Dial(wsURL+"?reject=1", nil)
	if err == nil {
		t.Fatal("expected dial to fail with reject=1")
	}
}

func TestKick(t *testing.T) {
	wsURL, controlURL := testEnv(t)

	conn := dialWS(t, wsURL, "id=kick-1")

	// Verify connection is alive.
	frame := map[string]any{"event": "hi"}
	if err := conn.WriteJSON(frame); err != nil {
		t.Fatalf("write: %v", err)
	}
	var got map[string]any
	if err := conn.ReadJSON(&got); err != nil {
		t.Fatalf("read: %v", err)
	}

	// Wait for server to register the connection in the hub.
	time.Sleep(100 * time.Millisecond)

	// Kick the connection.
	r := controlPost(t, controlURL, "/kick?id=kick-1")
	if !r.OK {
		t.Fatalf("kick: %s", r.Error)
	}

	// The WebSocket read should fail.
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Fatal("expected read to fail after kick")
	}
}

func TestKick_MissingID(t *testing.T) {
	_, controlURL := testEnv(t)

	resp, err := http.Post(controlURL+"/kick", "", nil)
	if err != nil {
		t.Fatalf("POST /kick: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400, got %d", resp.StatusCode)
	}
}

func TestKick_UnknownID(t *testing.T) {
	_, controlURL := testEnv(t)

	r := controlPost(t, controlURL, "/kick?id=nonexistent")
	if r.OK {
		t.Fatal("expected kick of unknown ID to fail")
	}
}

func TestShutdown(t *testing.T) {
	wsURL, controlURL := testEnv(t)

	// Verify WS is serving.
	conn := dialWS(t, wsURL, "id=sd-1")
	_ = conn.Close()

	// Shutdown.
	r := controlPost(t, controlURL, "/shutdown")
	if !r.OK {
		t.Fatalf("shutdown: %s", r.Error)
	}

	// Wait for listener to close.
	time.Sleep(100 * time.Millisecond)

	// New dials should fail.
	_, _, err := websocket.DefaultDialer.Dial(wsURL+"?id=sd-2", nil)
	if err == nil {
		t.Fatal("expected dial to fail after shutdown")
	}

	// Control port should still be alive.
	status := controlGet(t, controlURL, "/health")
	if status != http.StatusOK {
		t.Fatalf("health after shutdown: want 200, got %d", status)
	}
}

func TestShutdown_Idempotent(t *testing.T) {
	_, controlURL := testEnv(t)

	r := controlPost(t, controlURL, "/shutdown")
	if !r.OK {
		t.Fatalf("first shutdown: %s", r.Error)
	}

	r = controlPost(t, controlURL, "/shutdown")
	if !r.OK {
		t.Fatalf("second shutdown: %s", r.Error)
	}
}

func TestRestart(t *testing.T) {
	wsURL, controlURL := testEnv(t)

	// Shutdown first.
	r := controlPost(t, controlURL, "/shutdown")
	if !r.OK {
		t.Fatalf("shutdown: %s", r.Error)
	}

	time.Sleep(100 * time.Millisecond)

	// Verify dials fail.
	_, _, err := websocket.DefaultDialer.Dial(wsURL+"?id=rs-1", nil)
	if err == nil {
		t.Fatal("expected dial to fail after shutdown")
	}

	// Restart.
	r = controlPost(t, controlURL, "/restart")
	if !r.OK {
		t.Fatalf("restart: %s", r.Error)
	}

	time.Sleep(100 * time.Millisecond)

	// Verify WS is serving again on the same port.
	conn := dialWS(t, wsURL, "id=rs-2")

	frame := map[string]any{"event": "after-restart"}
	if err := conn.WriteJSON(frame); err != nil {
		t.Fatalf("write after restart: %v", err)
	}
	var got map[string]any
	if err := conn.ReadJSON(&got); err != nil {
		t.Fatalf("read after restart: %v", err)
	}
	if got["event"] != "after-restart" {
		t.Errorf("event: want %q, got %q", "after-restart", got["event"])
	}
}

func TestRestart_WhileRunning(t *testing.T) {
	wsURL, controlURL := testEnv(t)

	// Restart without shutting down first should still work.
	r := controlPost(t, controlURL, "/restart")
	if !r.OK {
		t.Fatalf("restart: %s", r.Error)
	}

	time.Sleep(100 * time.Millisecond)

	// Verify WS is serving.
	conn := dialWS(t, wsURL, "id=rr-1")

	frame := map[string]any{"event": "ok"}
	if err := conn.WriteJSON(frame); err != nil {
		t.Fatalf("write: %v", err)
	}
	var got map[string]any
	if err := conn.ReadJSON(&got); err != nil {
		t.Fatalf("read: %v", err)
	}
}

func TestRoomRouting(t *testing.T) {
	wsURL, _ := testEnv(t)

	conn := dialWS(t, wsURL, "room=myroom&id=room-1")

	frame := map[string]any{"event": "room-test"}
	if err := conn.WriteJSON(frame); err != nil {
		t.Fatalf("write: %v", err)
	}
	var got map[string]any
	if err := conn.ReadJSON(&got); err != nil {
		t.Fatalf("read: %v", err)
	}
	if got["event"] != "room-test" {
		t.Errorf("event: want %q, got %q", "room-test", got["event"])
	}
}

func TestKick_AfterShutdown(t *testing.T) {
	_, controlURL := testEnv(t)

	controlPost(t, controlURL, "/shutdown")

	resp, err := http.Post(controlURL+"/kick?id=x", "", nil)
	if err != nil {
		t.Fatalf("POST /kick: %v", err)
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", resp.StatusCode)
	}
}

func TestShutdownRestart_KickAfterRestart(t *testing.T) {
	wsURL, controlURL := testEnv(t)

	// Shutdown and restart.
	controlPost(t, controlURL, "/shutdown")
	time.Sleep(100 * time.Millisecond)

	r := controlPost(t, controlURL, "/restart")
	if !r.OK {
		t.Fatalf("restart: %s", r.Error)
	}
	time.Sleep(100 * time.Millisecond)

	// Connect and then kick.
	conn := dialWS(t, wsURL, "id=srk-1")

	frame := map[string]any{"event": "hi"}
	if err := conn.WriteJSON(frame); err != nil {
		t.Fatalf("write: %v", err)
	}
	var got map[string]any
	if err := conn.ReadJSON(&got); err != nil {
		t.Fatalf("read: %v", err)
	}

	r = controlPost(t, controlURL, "/kick?id=srk-1")
	if !r.OK {
		t.Fatalf("kick after restart: %s", r.Error)
	}

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Fatal("expected read to fail after kick")
	}
}

func TestFrameRoundTrip(t *testing.T) {
	wsURL, _ := testEnv(t)

	conn := dialWS(t, wsURL, "id=rt-1")

	outbound := map[string]any{
		"id":      "msg-001",
		"event":   "chat.message",
		"payload": map[string]any{"user": "alice", "text": "hello"},
	}
	if err := conn.WriteJSON(outbound); err != nil {
		t.Fatalf("write: %v", err)
	}

	var got map[string]any
	if err := conn.ReadJSON(&got); err != nil {
		t.Fatalf("read: %v", err)
	}

	if got["id"] != "msg-001" {
		t.Errorf("id: want %q, got %q", "msg-001", got["id"])
	}
	if got["event"] != "chat.message" {
		t.Errorf("event: want %q, got %q", "chat.message", got["event"])
	}
	payload, ok := got["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload: want map, got %T", got["payload"])
	}
	if payload["user"] != "alice" {
		t.Errorf("payload.user: want %q, got %q", "alice", payload["user"])
	}
}

func TestMultipleSendsOrdering(t *testing.T) {
	wsURL, _ := testEnv(t)

	conn := dialWS(t, wsURL, "id=ord-1")

	const count = 20
	for i := range count {
		frame := map[string]any{"event": "seq", "payload": map[string]any{"i": i}}
		if err := conn.WriteJSON(frame); err != nil {
			t.Fatalf("write #%d: %v", i, err)
		}
	}

	for i := range count {
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var got map[string]any
		if err := conn.ReadJSON(&got); err != nil {
			t.Fatalf("read #%d: %v", i, err)
		}
		payload := got["payload"].(map[string]any)
		gotIdx := int(payload["i"].(float64))
		if gotIdx != i {
			t.Errorf("order: want %d, got %d", i, gotIdx)
		}
	}
}

func TestConcurrentEcho(t *testing.T) {
	wsURL, _ := testEnv(t)

	const clients = 5
	errs := make(chan error, clients)

	for c := range clients {
		go func(clientNum int) {
			id := fmt.Sprintf("cc-%d", clientNum)
			url := wsURL + "?id=" + id

			conn, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				errs <- fmt.Errorf("dial %s: %w", id, err)
				return
			}
			defer func() { _ = conn.Close() }()

			frame := map[string]any{"event": "echo", "payload": id}
			if err := conn.WriteJSON(frame); err != nil {
				errs <- fmt.Errorf("write %s: %w", id, err)
				return
			}

			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			var got map[string]any
			if err := conn.ReadJSON(&got); err != nil {
				errs <- fmt.Errorf("read %s: %w", id, err)
				return
			}

			if got["payload"] != id {
				errs <- fmt.Errorf("%s: payload want %q, got %q", id, id, got["payload"])
				return
			}
			errs <- nil
		}(c)
	}

	for range clients {
		if err := <-errs; err != nil {
			t.Error(err)
		}
	}
}

func TestRejectDialError(t *testing.T) {
	wsURL, _ := testEnv(t)

	_, resp, err := websocket.DefaultDialer.Dial(wsURL+"?reject=1", nil)
	if err == nil {
		t.Fatal("expected dial error with reject=1")
	}
	if resp != nil && resp.StatusCode == http.StatusSwitchingProtocols {
		t.Fatal("expected non-101 status on reject")
	}

	// Verify the error message contains something useful.
	if !strings.Contains(err.Error(), "websocket") && !strings.Contains(err.Error(), "handshake") &&
		!strings.Contains(err.Error(), "bad") && !strings.Contains(err.Error(), "403") &&
		!strings.Contains(err.Error(), "500") && !strings.Contains(err.Error(), "401") {
		t.Logf("unexpected error format (still a failure): %v", err)
	}
}
