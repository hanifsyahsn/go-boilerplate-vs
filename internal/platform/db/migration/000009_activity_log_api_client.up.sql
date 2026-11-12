-- The machine caller of an API-key route (middleware.APIKey's client name, never the key), so a machine call
-- isn't recorded as an anonymous one. '' = not a machine call.
-- api_activity_logs is written on every request: give up after 5 s rather than wait behind a running write
-- (a cleanup DELETE) while every recorder INSERT queues behind this ALTER. Re-run it if it timed out.
-- SET LOCAL inside its own transaction: migrate runs a whole run on one connection, and a session SET would
-- reach the later migrations too.
BEGIN;
SET LOCAL lock_timeout = '5s';
ALTER TABLE api_activity_logs ADD COLUMN api_client varchar(100) NOT NULL DEFAULT '';
COMMIT;
