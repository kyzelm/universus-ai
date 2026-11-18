package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// A hub with no accounts, which is the configuration the room is specified in.
func relayServer(t *testing.T) string {
	t.Helper()
	return serverFor(t, &hub{rooms: &rooms{waiting: map[string]*client{}}, queue: newMatchmaker()})
}

func serverFor(t *testing.T, h *hub) string {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.serveRoom)
	mux.HandleFunc("/ws/queue", h.serveQueue)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

// dial opens a socket and sends the opening token frame every client sends, signed in or not.
func dial(t *testing.T, url string) *websocket.Conn {
	return dialAs(t, url, "")
}

func dialAs(t *testing.T, url, token string) *websocket.Conn {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	t.Cleanup(func() { conn.CloseNow() })

	if err := write(conn, websocket.MessageText, []byte(token)); err != nil {
		t.Fatalf("sending the token frame: %v", err)
	}
	return conn
}

func read(t *testing.T, conn *websocket.Conn) (websocket.MessageType, []byte) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	typ, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return typ, data
}

func send(t *testing.T, conn *websocket.Conn, typ websocket.MessageType, data []byte) {
	t.Helper()

	if err := write(conn, typ, data); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// The whole contract in one test.
func TestPairAndForward(t *testing.T) {
	url := relayServer(t) + "/ws?room=abc-1"

	host := dial(t, url)
	guest := dial(t, url)

	if _, role := read(t, host); string(role) != "host" {
		t.Errorf("first in the room got role %q, want host", role)
	}
	if _, role := read(t, guest); string(role) != "guest" {
		t.Errorf("second in the room got role %q, want guest", role)
	}

	// An offer blob: text, and the host sends it because it was told it is one.
	send(t, host, websocket.MessageText, []byte("offer-blob"))
	typ, data := read(t, guest)
	if typ != websocket.MessageText || string(data) != "offer-blob" {
		t.Errorf("guest read (%v, %q), want (text, offer-blob)", typ, data)
	}

	// An input packet on the way back: binary, and unchanged.
	packet := []byte{1, 0, 0, 0, 7, 2, 0x11, 0x00, 0x22, 0x00}
	send(t, guest, websocket.MessageBinary, packet)
	typ, data = read(t, host)
	if typ != websocket.MessageBinary || string(data) != string(packet) {
		t.Errorf("host read (%v, % x), want (binary, % x)", typ, data, packet)
	}
}

// Hanging up ends the match for both ends.
func TestPeerLeaving(t *testing.T) {
	url := relayServer(t) + "/ws?room=abc-1"

	host := dial(t, url)
	guest := dial(t, url)
	read(t, host)
	read(t, guest)

	host.CloseNow()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, data, err := guest.Read(ctx); err == nil {
		t.Errorf("guest read %q after the host left, want an error", data)
	}
}
