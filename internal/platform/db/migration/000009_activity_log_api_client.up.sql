-- The machine caller of an API-key route (middleware.APIKey's client name, never the key), so a machine call
-- isn't recorded as an anonymous one. '' = not a machine call.
ALTER TABLE api_activity_logs ADD COLUMN api_client varchar(100) NOT NULL DEFAULT '';
