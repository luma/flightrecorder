DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'events'
          AND column_name = 'player_id'
    ) THEN
        ALTER TABLE events RENAME COLUMN player_id TO commander_id;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'events_project_player_game_time_idx'
    ) THEN
        ALTER INDEX events_project_player_game_time_idx RENAME TO events_project_commander_game_time_idx;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'events_project_player_real_ts_idx'
    ) THEN
        ALTER INDEX events_project_player_real_ts_idx RENAME TO events_project_commander_real_ts_idx;
    END IF;

    UPDATE events
    SET event_json = (event_json - 'player_id') || jsonb_build_object('commander_id', event_json->'player_id')
    WHERE event_json ? 'player_id'
      AND NOT event_json ? 'commander_id';
END $$;
