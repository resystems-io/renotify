-- Schema V3: add flow context columns to notification_requests.
-- These are populated at insert time from the active flow record
-- so the history view can display flow label and workspace name
-- without joining against lifecycle tables.

ALTER TABLE notification_requests ADD COLUMN flow_label TEXT;
ALTER TABLE notification_requests ADD COLUMN workspace_name TEXT;
ALTER TABLE notification_requests ADD COLUMN workspace_path TEXT;
