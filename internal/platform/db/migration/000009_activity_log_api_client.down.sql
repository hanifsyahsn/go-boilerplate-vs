-- Same lock_timeout as the up file (api_activity_logs is written on every request).
BEGIN;
SET LOCAL lock_timeout = '5s';
ALTER TABLE api_activity_logs DROP COLUMN IF EXISTS api_client;
COMMIT;
