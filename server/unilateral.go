package main

// A match only one side ever reported on (02 Architecture/Disconnect and Match
// Integrity.md).

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	// How long a lone submission waits for its pair before it is accepted on
	// its own.
	unilateralGrace = 30 * time.Second

	// How often to look for one that has waited long enough.
	unilateralPoll = 10 * time.Second
)

// sweepUnilateral settles every match whose single submission has outlived the
// grace period.
func (v verifier) sweepUnilateral(ctx context.Context) bool {
	tx, err := v.matches.pool.Begin(ctx)
	if err != nil {
		log.Printf("unilateral: %v", err)
		return false
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// One row, locked, skipping anything another worker is already holding —
	// the same claim discipline the verification queue uses.
	var id int64
	err = tx.QueryRow(ctx,
		`SELECT m.id FROM matches m
		 JOIN match_submissions s ON s.match_id = m.id
		 WHERE m.winner IS NULL
		 GROUP BY m.id
		 HAVING count(*) = 1 AND min(s.received_at) < now() - $1::interval
		 ORDER BY m.id
		 LIMIT 1`, unilateralGrace.String()).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	if err != nil {
		log.Printf("unilateral: looking for one: %v", err)
		return false
	}

	// Re-read under a row lock before acting: between the aggregate above and
	// here, the other player's upload may have arrived and settled it.
	var row matchRow
	var winner *int
	if err := tx.QueryRow(ctx,
		`SELECT id, mode, p1_id, p2_id, winner FROM matches WHERE id = $1 FOR UPDATE`, id).
		Scan(&row.id, &row.mode, &row.p1, &row.p2, &winner); err != nil {
		log.Printf("unilateral: match %d: %v", id, err)
		return false
	}
	if winner != nil {
		return true // somebody else settled it; there may be more to do
	}

	var s submission
	if err := tx.QueryRow(ctx,
		`SELECT input_log, checksums, result FROM match_submissions WHERE match_id = $1`, id).
		Scan(&s.inputLog, &s.checksums, &s.outcome); err != nil {
		log.Printf("unilateral: reading the submission for %d: %v", id, err)
		return false
	}

	// Accepted as a disconnect whatever the client called it.
	s.Disconnect = true

	if err := v.matches.settleTx(ctx, tx, row, s); err != nil {
		log.Printf("unilateral: settling %d: %v", id, err)
		return false
	}
	if err := tx.Commit(ctx); err != nil {
		log.Printf("unilateral: committing %d: %v", id, err)
		return false
	}
	log.Printf("match %d settled on one report: seat %d won", id, s.Winner)
	return true
}

// runUnilateral drains the sweep on a slow tick until ctx ends.
func (v verifier) runUnilateral(ctx context.Context) {
	t := time.NewTicker(unilateralPoll)
	defer t.Stop()
	for {
		for v.sweepUnilateral(ctx) {
			if ctx.Err() != nil {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
