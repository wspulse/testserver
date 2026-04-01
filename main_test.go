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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err, "startWebSocket")

	ctlPort, err := ts.startControl()
	require.NoError(t, err, "startControl")

	t.Cleanup(ts.close)

	wsURL = fmt.Sprintf("ws://127.0.0.1:%d", wsPort)
	controlURL = fmt.Sprintf("http://127.0.0.1:%d", ctlPort)
	return wsURL, controlURL
}

// controlPost sends a POST to the control endpoint and returns the response.
func controlPost(t *testing.T, baseURL, path string) response {
	t.Helper()

	resp, err := http.Post(baseURL+path, "", nil)
	require.NoError(t, err, "POST %s", path)
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	var r response
	require.NoError(t, json.Unmarshal(body, &r), "decode response (body: %s)", body)
	return r
}

// controlGet sends a GET to the control endpoint and returns the HTTP status.
func controlGet(t *testing.T, baseURL, path string) int {
	t.Helper()

	resp, err := http.Get(baseURL + path)
	require.NoError(t, err, "GET %s", path)
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
	require.NoError(t, err, "dial %s", url)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// waitForDialFail polls until a WebSocket dial to wsURL fails, indicating
// the server is no longer accepting connections. It uses a 3-second deadline.
func waitForDialFail(t *testing.T, wsURL string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL+"?id=probe", nil)
		if err != nil {
			return // server is down — success
		}
		_ = conn.Close()
		time.Sleep(10 * time.Millisecond)
	}
	require.Fail(t, "timed out waiting for server to stop accepting connections")
}

// waitForDialSuccess polls until a WebSocket dial to wsURL succeeds,
// indicating the server is ready. It uses a 3-second deadline.
func waitForDialSuccess(t *testing.T, wsURL string) {
	t.Helper()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL+"?id=probe", nil)
		if err == nil {
			_ = conn.Close()
			return // server is up — success
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.Fail(t, "timed out waiting for server to accept connections")
}

// ── tests ───────────────────────────────────────────────────────────────────

func TestHealth(t *testing.T) {
	_, controlURL := testEnv(t)

	status := controlGet(t, controlURL, "/health")
	require.Equal(t, http.StatusOK, status, "health")
}

func TestEcho(t *testing.T) {
	wsURL, _ := testEnv(t)

	conn := dialWS(t, wsURL, "id=echo-1")

	frame := map[string]any{"event": "ping", "payload": "pong"}
	require.NoError(t, conn.WriteJSON(frame), "write")

	var got map[string]any
	require.NoError(t, conn.ReadJSON(&got), "read")

	assert.Equal(t, "ping", got["event"], "event")
}

func TestReject(t *testing.T) {
	wsURL, _ := testEnv(t)

	_, _, err := websocket.DefaultDialer.Dial(wsURL+"?reject=1", nil)
	require.Error(t, err, "expected dial to fail with reject=1")
}

func TestKick(t *testing.T) {
	wsURL, controlURL := testEnv(t)

	conn := dialWS(t, wsURL, "id=kick-1")

	// Verify connection is alive.
	frame := map[string]any{"event": "hi"}
	require.NoError(t, conn.WriteJSON(frame), "write")
	var got map[string]any
	require.NoError(t, conn.ReadJSON(&got), "read")

	// The echo round-trip above proves the connection is registered in the hub.

	// Kick the connection.
	r := controlPost(t, controlURL, "/kick?id=kick-1")
	require.True(t, r.OK, "kick: %s", r.Error)

	// The WebSocket read should fail.
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err := conn.ReadMessage()
	require.Error(t, err, "expected read to fail after kick")
}

func TestKick_MissingID(t *testing.T) {
	_, controlURL := testEnv(t)

	resp, err := http.Post(controlURL+"/kick", "", nil)
	require.NoError(t, err, "POST /kick")
	_ = resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestKick_UnknownID(t *testing.T) {
	_, controlURL := testEnv(t)

	r := controlPost(t, controlURL, "/kick?id=nonexistent")
	require.False(t, r.OK, "expected kick of unknown ID to fail")
}

func TestShutdown(t *testing.T) {
	wsURL, controlURL := testEnv(t)

	// Verify WS is serving.
	conn := dialWS(t, wsURL, "id=sd-1")
	_ = conn.Close()

	// Shutdown.
	r := controlPost(t, controlURL, "/shutdown")
	require.True(t, r.OK, "shutdown: %s", r.Error)

	// Wait for listener to close — poll until dial fails.
	waitForDialFail(t, wsURL)

	// Control port should still be alive.
	status := controlGet(t, controlURL, "/health")
	require.Equal(t, http.StatusOK, status, "health after shutdown")
}

func TestShutdown_Idempotent(t *testing.T) {
	_, controlURL := testEnv(t)

	r := controlPost(t, controlURL, "/shutdown")
	require.True(t, r.OK, "first shutdown: %s", r.Error)

	r = controlPost(t, controlURL, "/shutdown")
	require.True(t, r.OK, "second shutdown: %s", r.Error)
}

func TestRestart(t *testing.T) {
	wsURL, controlURL := testEnv(t)

	// Shutdown first.
	r := controlPost(t, controlURL, "/shutdown")
	require.True(t, r.OK, "shutdown: %s", r.Error)

	// Wait for listener to close — poll until dial fails.
	waitForDialFail(t, wsURL)

	// Restart.
	r = controlPost(t, controlURL, "/restart")
	require.True(t, r.OK, "restart: %s", r.Error)

	// Wait for server to be ready — poll until dial succeeds.
	waitForDialSuccess(t, wsURL)

	// Verify WS is serving again on the same port.
	conn := dialWS(t, wsURL, "id=rs-2")

	frame := map[string]any{"event": "after-restart"}
	require.NoError(t, conn.WriteJSON(frame), "write after restart")
	var got map[string]any
	require.NoError(t, conn.ReadJSON(&got), "read after restart")
	assert.Equal(t, "after-restart", got["event"], "event")
}

func TestRestart_WhileRunning(t *testing.T) {
	wsURL, controlURL := testEnv(t)

	// Restart without shutting down first should still work.
	r := controlPost(t, controlURL, "/restart")
	require.True(t, r.OK, "restart: %s", r.Error)

	// Wait for server to be ready — poll until dial succeeds.
	waitForDialSuccess(t, wsURL)

	// Verify WS is serving.
	conn := dialWS(t, wsURL, "id=rr-1")

	frame := map[string]any{"event": "ok"}
	require.NoError(t, conn.WriteJSON(frame), "write")
	var got map[string]any
	require.NoError(t, conn.ReadJSON(&got), "read")
}

func TestRoomRouting(t *testing.T) {
	wsURL, _ := testEnv(t)

	conn := dialWS(t, wsURL, "room=myroom&id=room-1")

	frame := map[string]any{"event": "room-test"}
	require.NoError(t, conn.WriteJSON(frame), "write")
	var got map[string]any
	require.NoError(t, conn.ReadJSON(&got), "read")
	assert.Equal(t, "room-test", got["event"], "event")
}

func TestKick_AfterShutdown(t *testing.T) {
	_, controlURL := testEnv(t)

	controlPost(t, controlURL, "/shutdown")

	resp, err := http.Post(controlURL+"/kick?id=x", "", nil)
	require.NoError(t, err, "POST /kick")
	_ = resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestShutdownRestart_KickAfterRestart(t *testing.T) {
	wsURL, controlURL := testEnv(t)

	// Shutdown and restart.
	controlPost(t, controlURL, "/shutdown")
	waitForDialFail(t, wsURL)

	r := controlPost(t, controlURL, "/restart")
	require.True(t, r.OK, "restart: %s", r.Error)
	waitForDialSuccess(t, wsURL)

	// Connect and then kick.
	conn := dialWS(t, wsURL, "id=srk-1")

	frame := map[string]any{"event": "hi"}
	require.NoError(t, conn.WriteJSON(frame), "write")
	var got map[string]any
	require.NoError(t, conn.ReadJSON(&got), "read")

	r = controlPost(t, controlURL, "/kick?id=srk-1")
	require.True(t, r.OK, "kick after restart: %s", r.Error)

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err := conn.ReadMessage()
	require.Error(t, err, "expected read to fail after kick")
}

func TestFrameRoundTrip(t *testing.T) {
	wsURL, _ := testEnv(t)

	conn := dialWS(t, wsURL, "id=rt-1")

	outbound := map[string]any{
		"id":      "msg-001",
		"event":   "chat.message",
		"payload": map[string]any{"user": "alice", "text": "hello"},
	}
	require.NoError(t, conn.WriteJSON(outbound), "write")

	var got map[string]any
	require.NoError(t, conn.ReadJSON(&got), "read")

	assert.Equal(t, "msg-001", got["id"], "id")
	assert.Equal(t, "chat.message", got["event"], "event")
	payload, ok := got["payload"].(map[string]any)
	require.True(t, ok, "payload: want map, got %T", got["payload"])
	assert.Equal(t, "alice", payload["user"], "payload.user")
}

func TestMultipleSendsOrdering(t *testing.T) {
	wsURL, _ := testEnv(t)

	conn := dialWS(t, wsURL, "id=ord-1")

	const count = 20
	for i := range count {
		frame := map[string]any{"event": "seq", "payload": map[string]any{"i": i}}
		require.NoError(t, conn.WriteJSON(frame), "write #%d", i)
	}

	for i := range count {
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		var got map[string]any
		require.NoError(t, conn.ReadJSON(&got), "read #%d", i)
		payload := got["payload"].(map[string]any)
		gotIdx := int(payload["i"].(float64))
		assert.Equal(t, i, gotIdx, "order")
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
		select {
		case err := <-errs:
			assert.NoError(t, err)
		case <-time.After(3 * time.Second):
			require.Fail(t, "timed out waiting for concurrent echo client to finish")
		}
	}
}

func TestRejectDialError(t *testing.T) {
	wsURL, _ := testEnv(t)

	_, resp, err := websocket.DefaultDialer.Dial(wsURL+"?reject=1", nil)
	require.Error(t, err, "expected dial error with reject=1")
	if resp != nil {
		require.NotEqual(t, http.StatusSwitchingProtocols, resp.StatusCode, "expected non-101 status on reject")
	}

	// Verify the error message contains something useful.
	if !strings.Contains(err.Error(), "websocket") && !strings.Contains(err.Error(), "handshake") &&
		!strings.Contains(err.Error(), "bad") && !strings.Contains(err.Error(), "403") &&
		!strings.Contains(err.Error(), "500") && !strings.Contains(err.Error(), "401") {
		t.Logf("unexpected error format (still a failure): %v", err)
	}
}
