-- A match row is created when two players are *paired*, which is before anybody
-- has chosen anything: the host decides the characters over the room and the
-- server relays that text without reading it (D88, D106), so the columns are
-- not known until the result is uploaded.
ALTER TABLE matches
    ALTER COLUMN p1_character DROP NOT NULL,
    ALTER COLUMN p2_character DROP NOT NULL,
    ALTER COLUMN rng_seed     DROP NOT NULL;

-- rng_seed stays null until the sim takes a per-match seed at all.
COMMENT ON COLUMN matches.rng_seed IS
    'null until matches carry their own seed; the sim uses one fixed seed today';
