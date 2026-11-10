// Universus backend.
//
//	GET /ws?room=CODE
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"universus/data"
)

const (
	// Generous for a base64 SDP blob with a long candidate list, mean for
	// anything else.
	readLimit = 32 << 10

	// A peer whose socket has stopped draining must not wedge the other end's
	// read loop.
	writeTimeout = 5 * time.Second

	// How long a client has to send its opening token frame.
	handshakeTimeout = 10 * time.Second
)

// hub is the server's shared state: the codes people type at each other, the
// queues people wait in, and the accounts both are gated on when there are any.
type hub struct {
	rooms *rooms
	queue *matchmaker
	// nil when the server runs without a database, which is also when it runs
	// without accounts.
	auth *auth
}

// Room codes are typed by hand and appear in a URL, so they are checked rather
// than trusted.
var codeOK = regexp.MustCompile(`^[A-Za-z0-9-]{1,32}$`)

type client struct {
	conn *websocket.Conn
	// Carries the peer to a client that had to wait for one.
	paired chan *websocket.Conn

	// Who this is, or nil when the server is running without accounts.
	user *User
	// Ladder points, read once when the queue is joined.
	lp int
	// Round trip to *this server* in nanoseconds, which is the only latency
	// anybody can measure before a match exists (02 Architecture/Backend
	// Services.md).
	rtt atomic.Int64
}

func newClient(conn *websocket.Conn) *client {
	return &client{conn: conn, paired: make(chan *websocket.Conn, 1)}
}

type rooms struct {
	mu      sync.Mutex
	waiting map[string]*client
}

// join registers c under code and returns the client already waiting there, if
// there was one — in which case the code is freed again, since the pair now
// talk to each other and not to the room.
// ponytail: a code holds one waiting client, not a two-seat room.
func (r *rooms) join(code string, c *client) *client {
	r.mu.Lock()
	defer r.mu.Unlock()

	if other, ok := r.waiting[code]; ok {
		delete(r.waiting, code)
		return other
	}
	r.waiting[code] = c
	return nil
}

func (r *rooms) leave(code string, c *client) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Only if it is still us: by now the code may hold somebody else.
	if r.waiting[code] == c {
		delete(r.waiting, code)
	}
}

// accept upgrades the request and reads the client's opening frame, which is
// its token or an empty string.
func (s *hub) accept(w http.ResponseWriter, req *http.Request) (*websocket.Conn, *User, bool) {
	conn, err := websocket.Accept(w, req, &websocket.AcceptOptions{
		// Input packets are 20 bytes of bitfields and do not compress; agreeing
		// on compression costs handshake and per-message work for nothing.
		CompressionMode: websocket.CompressionDisabled,
		// The credential is a frame rather than a cookie, so a page on another
		// origin has nothing to borrow by opening this socket.
		OriginPatterns: []string{"*"},
	})
	if err != nil {
		return nil, nil, false // Accept has already written the failure
	}
	conn.SetReadLimit(readLimit)

	ctx, cancel := context.WithTimeout(context.Background(), handshakeTimeout)
	defer cancel()

	typ, data, err := conn.Read(ctx)
	if err != nil || typ != websocket.MessageText {
		conn.Close(websocket.StatusPolicyViolation, "expected a token frame first")
		return nil, nil, false
	}

	if s.auth == nil {
		return conn, nil, true // no accounts on this server: nothing to check
	}

	u, err := s.auth.userOf(string(data))
	if err != nil {
		// D104: a private match by code needs an account exactly as a ranked
		// one does.
		conn.Close(websocket.StatusPolicyViolation, "sign in first")
		return nil, nil, false
	}
	return conn, &u, true
}

func (s *hub) serveRoom(w http.ResponseWriter, req *http.Request) {
	code := req.URL.Query().Get("room")
	if !codeOK.MatchString(code) {
		http.Error(w, "room code must be 1-32 characters of [A-Za-z0-9-]", http.StatusBadRequest)
		return
	}

	conn, user, ok := s.accept(w, req)
	if !ok {
		return
	}
	defer conn.CloseNow()

	me := newClient(conn)
	me.user = user

	if other := s.rooms.join(code, me); other != nil {
		if !pair(other, me, 0) {
			return
		}
	} else {
		defer s.rooms.leave(code, me)
	}
	relay(conn, me)
}

// pair introduces two clients to each other: roles out first, then each is
// handed the other's socket.
func pair(host, guest *client, matchID int64) bool {
	suffix := ""
	if matchID != 0 {
		suffix = " " + strconv.FormatInt(matchID, 10)
	}
	if tell(host.conn, "host"+suffix) != nil || tell(guest.conn, "guest"+suffix) != nil {
		return false
	}
	host.paired <- guest.conn
	guest.paired <- host.conn
	return true
}

// relay copies every frame to the peer and reads none of them.
func relay(conn *websocket.Conn, me *client) {
	// Resolves the peer once, whether the pairing happened on this goroutine or
	// arrived while we were waiting for somebody to join.
	var peer *websocket.Conn
	peerOf := func() *websocket.Conn {
		if peer == nil {
			select {
			case p := <-me.paired:
				peer = p
			default:
			}
		}
		return peer
	}

	// Hanging up on the peer is what tells the other end the match is over.
	defer func() {
		if p := peerOf(); p != nil {
			p.CloseNow()
		}
	}()

	// Not req.Context(): reads end when the socket does, which is the only
	// signal that matters here.
	ctx := context.Background()

	for {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			return
		}

		p := peerOf()
		if p == nil {
			continue // nobody to copy to yet, so this was noise
		}

		if err := write(p, typ, data); err != nil {
			return
		}
	}
}

func tell(conn *websocket.Conn, role string) error {
	return write(conn, websocket.MessageText, []byte(role))
}

func write(conn *websocket.Conn, typ websocket.MessageType, data []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	return conn.Write(ctx, typ, data)
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	static := flag.String("static", "", "directory of built client to serve at /; empty serves signalling only")
	flag.Parse()

	h := &hub{rooms: &rooms{waiting: map[string]*client{}}, queue: newMatchmaker()}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.serveRoom)

	// The database is optional and the server starts without one.
	ctx := context.Background()
	pool, err := openDB(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	if pool != nil {
		defer pool.Close()
		// Migrations run at startup, in the binary that needs them: a container
		// that starts is a container that has its schema (02 Architecture/
		// Database Schema.md).
		n, err := migrate(ctx, pool, migrationFiles)
		if err != nil {
			log.Fatalf("migrating: %v", err)
		}
		log.Printf("database ready, %d migration(s) applied", n)

		h.auth = &auth{
			users:   pgStore{pool: pool},
			secret:  jwtSecret(os.Getenv("UNIVERSUS_JWT_SECRET")),
			matches: &matchStore{pool: pool},
		}
		h.queue.matches = h.auth.matches
		h.auth.routes(mux)

		// Matchmaking exists only where accounts do: a queue is an ordering of
		// ladder ratings, and there are none without a database (D104).
		mux.HandleFunc("/ws/queue", h.serveQueue)
		go h.queue.run(ctx)

		// The roster this binary re-simulates with.
		if err := data.Install(); err != nil {
			log.Fatalf("game data: %v", err)
		}

		version, err := data.Version()
		if err != nil {
			log.Fatalf("hashing the character data: %v", err)
		}
		v := verifier{matches: *h.auth.matches, dataVersion: version}
		v.run(ctx, verifyWorkers)
		// A match only one side ever reported on is settled on that report, but
		// not immediately: the grace period is what stops whichever packet
		// arrived first from deciding the match (D52).
		go v.runUnilateral(ctx)

		log.Printf("accounts, matchmaking and verification enabled (data %08x)", version)
	} else {
		log.Printf("no DATABASE_URL: offline modes only, no accounts, no queue")
	}

	if *static != "" {
		// One origin for the page and the socket, which is what a remote
		// session needs: a guest on https cannot open a ws:// socket to a
		// separate port, and a tunnel or a host publishes one port.
		mux.Handle("/", http.FileServer(http.Dir(*static)))
		log.Printf("serving %s at /", *static)
	}

	log.Printf("relay listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}
