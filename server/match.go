// Match results and what they do to the ladder (02 Architecture/Anti-Cheat and
// Verification.md, 03 Game Design/Game Modes.md).
//
//	POST /api/match/{id}/result
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// A match is under 20 KB packed (02 Architecture/WASM Boundary.md).
const maxUpload int64 = 256 << 10

// outcome is what a client reports about a match, stored as the submission's JSONB.
type outcome struct {
	// 1 or 2 — the seat, not the account.
	Winner     int    `json:"winner"`
	EndFrame   int    `json:"endFrame"`
	Chars      [2]int `json:"chars"`
	Disconnect bool   `json:"disconnect"`

	// Measurement for the P2P-versus-relay comparison in the results chapter.
	Transport   string  `json:"transport"`
	RTTMs       int     `json:"rttMs"`
	RollbackAvg float64 `json:"rollbackAvg"`
}

// result is the upload body: the outcome plus the replay log and checksums in base64.
type result struct {
	outcome
	InputLog  string `json:"inputLog"`
	Checksums string `json:"checksums"`
}

// submission is one client's upload as stored.
type submission struct {
	outcome
	inputLog, checksums []byte
}

// matchRow is what the server knows about a match before anybody reports on it.
type matchRow struct {
	id     int64
	mode   string
	p1, p2 int64
	winner *int
}

type matchStore struct{ pool *pgxpool.Pool }

// create records a pairing.
func (m matchStore) create(ctx context.Context, mode string, p1, p2 int64) (int64, error) {
	var id int64
	err := m.pool.QueryRow(ctx,
		`INSERT INTO matches (mode, p1_id, p2_id) VALUES ($1, $2, $3) RETURNING id`,
		mode, p1, p2).Scan(&id)
	return id, err
}

func (m matchStore) byID(ctx context.Context, id int64) (matchRow, error) {
	var r matchRow
	err := m.pool.QueryRow(ctx,
		`SELECT id, mode, p1_id, p2_id, winner FROM matches WHERE id = $1`, id).
		Scan(&r.id, &r.mode, &r.p1, &r.p2, &r.winner)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, errNoMatch
	}
	return r, err
}

var (
	errNoMatch     = errors.New("no such match")
	errNotYours    = errors.New("not a player in that match")
	errAlreadySent = errors.New("already submitted")
	errSettled     = errors.New("that match is already settled")
)

// record stores one player's upload and returns the other player's, or nil when
// theirs has not arrived yet.
func (m matchStore) record(ctx context.Context, id, user int64, s submission) (*submission, error) {
	_, err := m.pool.Exec(ctx,
		`INSERT INTO match_submissions (match_id, user_id, input_log, checksums, result)
		 VALUES ($1, $2, $3, $4, $5)`,
		id, user, s.inputLog, s.checksums, s.outcome)
	if uniqueViolation(err) {
		return nil, errAlreadySent
	}
	if err != nil {
		return nil, err
	}

	var other submission
	err = m.pool.QueryRow(ctx,
		`SELECT input_log, checksums, result FROM match_submissions
		 WHERE match_id = $1 AND user_id <> $2`, id, user).
		Scan(&other.inputLog, &other.checksums, &other.outcome)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // theirs has not arrived
	}
	if err != nil {
		return nil, err
	}
	return &other, nil
}

// settle writes the agreed result, applies the ladder, and queues verification.
func (m matchStore) settle(ctx context.Context, r matchRow, s submission) error {
	return pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error { return m.settleTx(ctx, tx, r, s) })
}

// settleTx is the same work inside a transaction somebody else owns — the
// unilateral sweep claims its match and settles it in one.
func (m matchStore) settleTx(ctx context.Context, tx pgx.Tx, r matchRow, s submission) error {
	// Casual is accepted as reported, which is what "no verification" means —
	// and it is what makes ranked and casual genuinely different systems rather
	// than two buttons on a menu (D19).
	verified := "ok"
	if r.mode == "ranked" {
		verified = "pending"
	}

	// nullable() rather than a zero: a match nobody measured and a match
	// measured at zero milliseconds are different claims, and only one of them
	// belongs in a results chapter.
	if _, err := tx.Exec(ctx,
		`UPDATE matches SET winner = $2, end_frame = $3, p1_character = $4, p2_character = $5,
		        verified = $6, disconnected = $7, transport = $8, avg_rtt_ms = $9,
		        rollback_avg = $10 WHERE id = $1`,
		r.id, s.Winner, s.EndFrame, s.Chars[0], s.Chars[1], verified, s.Disconnect,
		nullable(s.Transport != "", s.Transport),
		nullable(s.RTTMs > 0, s.RTTMs),
		nullable(s.RollbackAvg > 0, s.RollbackAvg)); err != nil {
		return err
	}

	if r.mode == "ranked" {
		// The amounts are recorded with the match, because the verification
		// worker reverses exactly what was paid and cannot recompute it once
		// the ratings have moved (migration 003).
		p1Change, p2Change, err := applyLadder(ctx, tx, r, s.Winner)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`UPDATE matches SET p1_lp_change = $2, p2_lp_change = $3 WHERE id = $1`,
			r.id, p1Change, p2Change); err != nil {
			return err
		}
		// The job the re-simulation will pick up.
		if _, err := tx.Exec(ctx,
			`INSERT INTO verification_jobs (match_id) VALUES ($1) ON CONFLICT DO NOTHING`,
			r.id); err != nil {
			return err
		}
	}
	if s.Disconnect {
		return recordDisconnect(ctx, tx, r, s.Winner)
	}
	return nil
}

// nullable writes a value or SQL NULL.
func nullable[T any](ok bool, v T) any {
	if !ok {
		return nil
	}
	return v
}

// players is the match's two accounts, winner first, for a 1-based winning seat.
func (r matchRow) players(winnerSeat int) (winner, loser int64) {
	if winnerSeat == 2 {
		return r.p2, r.p1
	}
	return r.p1, r.p2
}

// recordDisconnect keeps the two counts the policy rests on.
func recordDisconnect(ctx context.Context, tx pgx.Tx, r matchRow, winnerSeat int) error {
	winner, loser := r.players(winnerSeat)
	if _, err := tx.Exec(ctx,
		`UPDATE ratings SET disconnects = disconnects + 1 WHERE user_id = $1`, loser); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`UPDATE ratings SET unilateral_wins = unilateral_wins + 1 WHERE user_id = $1`, winner)
	return err
}

// applyLadder moves both ratings.
func applyLadder(ctx context.Context, tx pgx.Tx, r matchRow, winnerSeat int) (int, int, error) {
	winner, loser := r.players(winnerSeat)

	// Locked in a fixed order — lowest id first — because two matches settling
	// at once on overlapping players is exactly how a deadlock is written.
	first, second := winner, loser
	if second < first {
		first, second = second, first
	}
	lp := map[int64]int{}
	rows, err := tx.Query(ctx,
		`SELECT user_id, lp FROM ratings WHERE user_id IN ($1, $2) ORDER BY user_id FOR UPDATE`,
		first, second)
	if err != nil {
		return 0, 0, err
	}
	for rows.Next() {
		var id, points int
		if err := rows.Scan(&id, &points); err != nil {
			rows.Close()
			return 0, 0, err
		}
		lp[int64(id)] = points
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	change := lpChange(lp[winner], lp[loser])
	newWinner := lp[winner] + change
	newLoser := applyLoss(lp[loser], change)

	for _, u := range []struct {
		id      int64
		lp, won int
	}{
		{winner, newWinner, 1},
		{loser, newLoser, 0},
	} {
		if _, err := tx.Exec(ctx,
			`UPDATE ratings SET lp = $2, tier = $3, matches = matches + 1, wins = wins + $4,
			        updated_at = now() WHERE user_id = $1`,
			u.id, u.lp, tierOf(u.lp), u.won); err != nil {
			return 0, 0, err
		}
	}

	// Signed, per seat, and not the same magnitude: the loser's is floored at
	// their tier boundary, so they may have paid less than the winner gained.
	changes := map[int64]int{winner: newWinner - lp[winner], loser: newLoser - lp[loser]}
	return changes[r.p1], changes[r.p2], nil
}

// void is what a disagreement costs: no winner, no LP, and both accounts
// flagged for a person to look at.
func (m matchStore) void(ctx context.Context, r matchRow) error {
	return pgx.BeginFunc(ctx, m.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE matches SET verified = 'mismatch' WHERE id = $1`, r.id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE users SET flagged = true WHERE id IN ($1, $2)`, r.p1, r.p2)
		return err
	})
}

func (a *auth) submitResult(w http.ResponseWriter, r *http.Request, u User) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		httpError(w, http.StatusBadRequest, "that is not a match id")
		return
	}

	var in result
	if !readBigJSON(w, r, &in) {
		return
	}
	s, err := in.decode()
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx := r.Context()
	match, err := a.matches.byID(ctx, id)
	if err != nil {
		// A match that does not exist and one somebody else played are the same
		// answer: otherwise this endpoint counts how many matches have been
		// played, one probe at a time.
		httpError(w, http.StatusNotFound, "no such match")
		return
	}
	if u.ID != match.p1 && u.ID != match.p2 {
		httpError(w, http.StatusNotFound, "no such match")
		return
	}
	if match.winner != nil {
		httpError(w, http.StatusConflict, "that match is already settled")
		return
	}

	other, err := a.matches.record(ctx, id, u.ID, s)
	if errors.Is(err, errAlreadySent) {
		httpError(w, http.StatusConflict, "you have already uploaded this match")
		return
	}
	if err != nil {
		log.Printf("recording match %d: %v", id, err)
		httpError(w, http.StatusInternalServerError, "could not store the result")
		return
	}

	if other == nil {
		// One upload is not a result.
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "waiting for the other player"})
		return
	}

	if !agree(s, *other) {
		if err := a.matches.void(ctx, match); err != nil {
			log.Printf("voiding match %d: %v", id, err)
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "mismatch"})
		return
	}

	if err := a.matches.settle(ctx, match, s); err != nil {
		log.Printf("settling match %d: %v", id, err)
		httpError(w, http.StatusInternalServerError, "could not settle the match")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "settled"})
}

// agree is check 1: do the two uploaded input logs match?
func agree(a, b submission) bool {
	return a.Winner == b.Winner && a.EndFrame == b.EndFrame && a.Chars == b.Chars &&
		bytes.Equal(a.inputLog, b.inputLog) && bytes.Equal(a.checksums, b.checksums)
}

func (in result) decode() (submission, error) {
	if in.Winner != 1 && in.Winner != 2 {
		return submission{}, errors.New("winner must be 1 or 2")
	}
	if in.EndFrame <= 0 {
		return submission{}, errors.New("endFrame must be positive")
	}
	inputLog, err := base64.StdEncoding.DecodeString(in.InputLog)
	if err != nil || len(inputLog) == 0 {
		return submission{}, errors.New("inputLog must be base64 and not empty")
	}
	checksums, err := base64.StdEncoding.DecodeString(in.Checksums)
	if err != nil || len(checksums)%4 != 0 {
		return submission{}, errors.New("checksums must be base64 uint32s")
	}
	if in.Transport != "" && in.Transport != "p2p" && in.Transport != "relay" {
		return submission{}, errors.New("transport must be p2p or relay")
	}
	return submission{in.outcome, inputLog, checksums}, nil
}
