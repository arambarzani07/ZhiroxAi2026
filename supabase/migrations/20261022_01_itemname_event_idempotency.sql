-- PRE-DEPLOYMENT CANDIDATE ONLY. Do not apply until isolated staging tests pass.
-- Adds idempotency metadata to relay events; does not touch legacy SQL Server.
ALTER TABLE public.pc_os_events
  ADD COLUMN IF NOT EXISTS event_key text;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint
    WHERE conname = 'pc_os_events_event_key_format'
      AND conrelid = 'public.pc_os_events'::regclass
  ) THEN
    ALTER TABLE public.pc_os_events ADD CONSTRAINT pc_os_events_event_key_format
      CHECK (event_key IS NULL OR event_key ~ '^[0-9a-f]{64}$');
  END IF;
END $$;

-- Null event keys for other existing event types retain their old behavior.
-- Duplicates of an ItemName event from the same agent get SQLSTATE 23505.
CREATE UNIQUE INDEX IF NOT EXISTS pc_os_events_agent_event_key_unique
  ON public.pc_os_events (agent_id, event_type, event_key)
  WHERE event_key IS NOT NULL;

-- Deployment order: apply migration, deploy authenticated event handler,
-- then run the same-event-twice ingestion test with a disposable test agent.
-- The production handler must return HTTP 202 with duplicate=true on a
-- collision with pc_os_events_agent_event_key_unique.
