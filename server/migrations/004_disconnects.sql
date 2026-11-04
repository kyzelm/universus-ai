-- A disconnect is a loss for the disconnecting player, and the only thing that
-- separates a rage-quit from a pulled cable is statistics over many matches (02
-- Architecture/Disconnect and Match Integrity.md).
ALTER TABLE ratings
    ADD COLUMN disconnects      INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN unilateral_wins  INTEGER NOT NULL DEFAULT 0;

COMMENT ON COLUMN ratings.disconnects IS
    'matches that ended with this account not reporting; a review signal, never an automatic penalty';
COMMENT ON COLUMN ratings.unilateral_wins IS
    'wins that arrived because the opponent stopped reporting; disproportionate is what gets flagged';

-- How the match ended, and on what.
ALTER TABLE matches
    ADD COLUMN disconnected BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN matches.disconnected IS
    'the match ended because one side stopped sending, rather than by KO or timeout';
