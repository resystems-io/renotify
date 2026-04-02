-- Schema V2: add workspace display columns to active_flows.
-- These are daemon-side enrichment fields (not part of the NATS
-- wire format) needed for the heartbeat workspace snapshot.

ALTER TABLE active_flows ADD COLUMN display_name TEXT;
ALTER TABLE active_flows ADD COLUMN abs_path TEXT;
