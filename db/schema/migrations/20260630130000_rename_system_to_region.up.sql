DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'events'
          AND column_name = 'system_id'
    ) THEN
        ALTER TABLE events RENAME COLUMN system_id TO region_id;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'events_project_system_zone_idx'
    ) THEN
        ALTER INDEX events_project_system_zone_idx RENAME TO events_project_region_zone_idx;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'events_project_system_time_idx'
    ) THEN
        ALTER INDEX events_project_system_time_idx RENAME TO events_project_region_time_idx;
    END IF;

    IF EXISTS (
        SELECT 1
        FROM pg_class
        WHERE relname = 'events_project_system_zone_time_idx'
    ) THEN
        ALTER INDEX events_project_system_zone_time_idx RENAME TO events_project_region_zone_time_idx;
    END IF;

    -- Align stored raw events with the renamed wire fields
    -- (context.location.world_id -> region_id, area_id -> zone_id).
    UPDATE events
    SET event_json = jsonb_set(
        event_json #- '{context,location,world_id}',
        '{context,location,region_id}',
        event_json #> '{context,location,world_id}'
    )
    WHERE event_json #> '{context,location,world_id}' IS NOT NULL
      AND event_json #> '{context,location,region_id}' IS NULL;

    UPDATE events
    SET event_json = jsonb_set(
        event_json #- '{context,location,area_id}',
        '{context,location,zone_id}',
        event_json #> '{context,location,area_id}'
    )
    WHERE event_json #> '{context,location,area_id}' IS NOT NULL
      AND event_json #> '{context,location,zone_id}' IS NULL;
END $$;
