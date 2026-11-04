-- What the ladder actually paid, per seat, signed.
ALTER TABLE matches
    ADD COLUMN p1_lp_change INTEGER,
    ADD COLUMN p2_lp_change INTEGER;
