// Matchmaking (02 Architecture/Backend Services.md).
//
//	GET /ws/queue?mode=ranked|casual
package main

import (
	"context"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// The bands, straight from the design note's table.
const unlimited = math.MaxInt

var bands = []struct {
	after   time.Duration
	ranked  int // LP difference allowed
	casual  int
	maxPing time.Duration
}{
	{after: 0, ranked: 100, casual: 500, maxPing: 60 * time.Millisecond},
	{after: 15 * time.Second, ranked: 250, casual: 1500, maxPing: 100 * time.Millisecond},
	{after: 30 * time.Second, ranked: 500, casual: unlimited, maxPing: 150 * time.Millisecond},
	{after: 60 * time.Second, ranked: unlimited, casual: unlimited, maxPing: math.MaxInt64},
}

// Re-evaluated on this interval, and on every join.
const sweepEvery = 5 * time.Second

// bandFor is the widest band either player has earned.
func bandFor(mode string, waited time.Duration) (lp int, maxPing time.Duration) {
	b := bands[0]
	for _, row := range bands {
		if waited >= row.after {
			b = row
		}
	}
	if mode == "ranked" {
		return b.ranked, b.maxPing
	}
	return b.casual, b.maxPing
}

type waiting struct {
	c      *client
	joined time.Time
}

type matchmaker struct {
	mu    sync.Mutex
	modes map[string][]*waiting

	// Where a pairing is recorded, or nil on a server with no database — in
	// which case there is no queue either, so this is nil only in tests that
	// exercise the pairing rule without one.
	matches *matchStore
}

func newMatchmaker() *matchmaker {
	return &matchmaker{modes: map[string][]*waiting{}}
}

// run sweeps the queues until ctx ends, so a player whose band has widened is
// matched without anybody else having to arrive.
func (m *matchmaker) run(ctx context.Context) {
	t := time.NewTicker(sweepEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.sweep(time.Now())
		}
	}
}

func (m *matchmaker) enqueue(mode string, c *client) *waiting {
	w := &waiting{c: c, joined: time.Now()}

	m.mu.Lock()
	m.modes[mode] = append(m.modes[mode], w)
	m.mu.Unlock()

	// Try at once: two people in a queue should not wait for a tick.
	m.sweep(time.Now())
	return w
}

// leave takes a client out, whether it was matched or its socket died.
func (m *matchmaker) leave(mode string, w *waiting) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.remove(mode, w)
}

// size is how many are waiting in a mode.
func (m *matchmaker) size(mode string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.modes[mode])
}

func (m *matchmaker) remove(mode string, w *waiting) {
	q := m.modes[mode]
	for i, x := range q {
		if x == w {
			m.modes[mode] = append(q[:i], q[i+1:]...)
			return
		}
	}
}

// sweep pairs everyone it can, oldest waiter first.
func (m *matchmaker) sweep(now time.Time) {
	type made struct {
		mode string
		a, b *waiting
	}

	// Chosen under the lock, introduced outside it.
	var pairs []made
	m.mu.Lock()
	for mode := range m.modes {
		for {
			a, b := m.bestPair(mode, now)
			if a == nil {
				break
			}
			m.remove(mode, a)
			m.remove(mode, b)
			pairs = append(pairs, made{mode, a, b})
		}
	}
	m.mu.Unlock()

	for _, p := range pairs {
		m.introduce(p.mode, p.a, p.b)
	}
}

// introduce records the match and tells the two ends who they are.
func (m *matchmaker) introduce(mode string, a, b *waiting) {
	var id int64
	if m.matches != nil && a.c.user != nil && b.c.user != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		recorded, err := m.matches.create(ctx, mode, a.c.user.ID, b.c.user.ID)
		cancel()
		if err != nil {
			// The match is still playable; it just will not count.
			log.Printf("recording a %s pairing: %v", mode, err)
		}
		id = recorded
	}
	pair(a.c, b.c, id)
}

func (m *matchmaker) bestPair(mode string, now time.Time) (*waiting, *waiting) {
	q := m.modes[mode]
	if len(q) < 2 {
		return nil, nil
	}

	oldest := q[0]
	for _, w := range q {
		if w.joined.Before(oldest.joined) {
			oldest = w
		}
	}

	var best *waiting
	for _, w := range q {
		if w == oldest || !acceptable(mode, now, oldest, w) {
			continue
		}
		if best == nil || lpGap(oldest, w) < lpGap(oldest, best) {
			best = w
		}
	}
	if best == nil {
		return nil, nil
	}
	return oldest, best
}

func lpGap(a, b *waiting) int {
	gap := a.c.lp - b.c.lp
	if gap < 0 {
		return -gap
	}
	return gap
}

// acceptable applies the band the longer waiter has earned.
func acceptable(mode string, now time.Time, a, b *waiting) bool {
	waited := max(now.Sub(a.joined), now.Sub(b.joined))
	lp, maxPing := bandFor(mode, waited)

	if lp != unlimited && lpGap(a, b) > lp {
		return false
	}
	// An unmeasured round trip passes: a client whose ping we never got back is
	// not evidence of a bad connection, and refusing it would make a lost pong
	// into an unmatchable player.
	ra, rb := a.c.rtt.Load(), b.c.rtt.Load()
	if ra > 0 && rb > 0 && time.Duration(ra+rb) > maxPing {
		return false
	}
	return true
}

func (s *hub) serveQueue(w http.ResponseWriter, req *http.Request) {
	mode := req.URL.Query().Get("mode")
	if mode != "ranked" && mode != "casual" {
		http.Error(w, "mode must be ranked or casual", http.StatusBadRequest)
		return
	}

	// Queueing needs an account, in both modes (D104, 03 Game Design/Game
	// Modes.md).
	conn, user, ok := s.accept(w, req)
	if !ok {
		return
	}
	defer conn.CloseNow()

	me := newClient(conn)
	me.user = user
	if user != nil {
		me.lp = s.auth.users.ratingOf(req.Context(), user.ID)
	}

	queued := s.queue.enqueue(mode, me)
	defer s.queue.leave(mode, queued)

	// Beside the relay loop, not before it.
	go measureRTT(conn, me)

	relay(conn, me)
}

// measureRTT times one WebSocket ping into the client's rtt field.
func measureRTT(conn *websocket.Conn, c *client) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	if err := conn.Ping(ctx); err != nil {
		return // unmeasured, which acceptable() reads as "no evidence"
	}
	c.rtt.Store(int64(time.Since(start)))
}
