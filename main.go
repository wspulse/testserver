// Package main implements a shared wspulse test server for non-Go client
// integration tests. It exposes two local ports:
//
//   - WebSocket port — echo server with query-param-controlled behaviour:
//     ?reject=1  → ConnectFunc returns an error (HTTP 401)
//     ?room=<id> → assigns connection to room <id> (default: "test")
//     ?id=<id>   → sets connectionID (default: auto-generated UUID)
//
//   - Control port — HTTP API for test orchestration:
//     GET  /health   → 200 OK
//     POST /kick     → kick a connection by ?id=<connectionID>
//     POST /shutdown → close WebSocket server + listener
//     POST /restart  → restart WebSocket server on the same port
//
// On startup, the server prints "READY:<ws_port>:<control_port>" to stderr.
// Client test harnesses parse this line to discover both ports.
package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"go.uber.org/zap"

	wspulse "github.com/wspulse/server"
)

func main() {
	logger := zap.Must(zap.NewDevelopment())

	ts := newTestServer(logger)

	wsPort, err := ts.startWebSocket()
	if err != nil {
		logger.Fatal("failed to start WebSocket listener", zap.Error(err))
	}

	controlPort, err := ts.startControl()
	if err != nil {
		logger.Fatal("failed to start control listener", zap.Error(err))
	}

	fmt.Fprintf(os.Stderr, "READY:%d:%d\n", wsPort, controlPort)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	logger.Info("shutting down")
	ts.close()
}

// testServer encapsulates the dual-port server state.
type testServer struct {
	logger *zap.Logger

	mu         sync.Mutex
	server     wspulse.Server
	wsListener net.Listener
	wsPort     int // fixed port for restart
	wsServing  chan struct{}
}

func newTestServer(logger *zap.Logger) *testServer {
	return &testServer{logger: logger}
}

func (ts *testServer) newWSServer() wspulse.Server {
	return wspulse.NewServer(
		func(r *http.Request) (roomID, connectionID string, err error) {
			if r.URL.Query().Get("reject") == "1" {
				return "", "", fmt.Errorf("rejected by test server")
			}
			room := r.URL.Query().Get("room")
			if room == "" {
				room = "test"
			}
			return room, r.URL.Query().Get("id"), nil
		},
		wspulse.WithOnMessage(func(connection wspulse.Connection, f wspulse.Frame) {
			if err := connection.Send(f); err != nil {
				ts.logger.Warn("echo send failed", zap.Error(err))
			}
		}),
		wspulse.WithLogger(ts.logger),
		wspulse.WithMaxMessageSize(1<<20),
	)
}

// startWebSocket creates the wspulse server and starts serving on an
// ephemeral port. Returns the port number.
func (ts *testServer) startWebSocket() (int, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.server = ts.newWSServer()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	ts.wsListener = ln
	ts.wsPort = ln.Addr().(*net.TCPAddr).Port
	ts.wsServing = make(chan struct{})

	srv := ts.server
	go func() {
		defer close(ts.wsServing)
		if err := http.Serve(ln, srv); err != nil {
			ts.logger.Debug("ws http.Serve exited", zap.Error(err))
		}
	}()

	return ts.wsPort, nil
}

// startControl starts the HTTP control server on an ephemeral port.
// Returns the port number.
func (ts *testServer) startControl() (int, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", ts.handleHealth)
	mux.HandleFunc("POST /kick", ts.handleKick)
	mux.HandleFunc("POST /shutdown", ts.handleShutdown)
	mux.HandleFunc("POST /restart", ts.handleRestart)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}

	go func() {
		if err := http.Serve(ln, mux); err != nil {
			ts.logger.Debug("control http.Serve exited", zap.Error(err))
		}
	}()

	return ln.Addr().(*net.TCPAddr).Port, nil
}

func (ts *testServer) close() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if ts.server != nil {
		ts.server.Close()
		ts.server = nil
	}
	if ts.wsListener != nil {
		_ = ts.wsListener.Close()
		<-ts.wsServing
		ts.wsListener = nil
	}
}

// ── control handlers ────────────────────────────────────────────────────────

func (ts *testServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, response{OK: true})
}

func (ts *testServer) handleKick(w http.ResponseWriter, r *http.Request) {
	connectionID := r.URL.Query().Get("id")
	if connectionID == "" {
		writeJSON(w, http.StatusBadRequest, response{OK: false, Error: "missing ?id= parameter"})
		return
	}

	ts.mu.Lock()
	srv := ts.server
	ts.mu.Unlock()

	if srv == nil {
		writeJSON(w, http.StatusServiceUnavailable, response{OK: false, Error: "server is shut down"})
		return
	}

	if err := srv.Kick(connectionID); err != nil {
		writeJSON(w, http.StatusBadRequest, response{OK: false, Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, response{OK: true})
}

func (ts *testServer) handleShutdown(w http.ResponseWriter, _ *http.Request) {
	ts.mu.Lock()

	if ts.server == nil {
		ts.mu.Unlock()
		writeJSON(w, http.StatusOK, response{OK: true, Error: "already shut down"})
		return
	}

	srv := ts.server
	ln := ts.wsListener
	serving := ts.wsServing
	ts.server = nil
	ts.wsListener = nil
	ts.mu.Unlock()

	srv.Close()
	if ln != nil {
		_ = ln.Close()
		<-serving
	}

	writeJSON(w, http.StatusOK, response{OK: true})
}

func (ts *testServer) handleRestart(w http.ResponseWriter, _ *http.Request) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Clean up previous instance if still running.
	if ts.server != nil {
		ts.server.Close()
	}
	if ts.wsListener != nil {
		_ = ts.wsListener.Close()
		<-ts.wsServing
	}

	ts.server = ts.newWSServer()

	// Rebind to the same port so client URLs remain valid.
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", ts.wsPort))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, response{OK: false, Error: err.Error()})
		return
	}
	ts.wsListener = ln
	ts.wsServing = make(chan struct{})

	srv := ts.server
	go func() {
		defer close(ts.wsServing)
		if err := http.Serve(ln, srv); err != nil {
			ts.logger.Debug("ws http.Serve exited (restart)", zap.Error(err))
		}
	}()

	writeJSON(w, http.StatusOK, response{OK: true})
}

// ── JSON helpers ────────────────────────────────────────────────────────────

type response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
