-- One row per /api request: who called which endpoint, when, how it went. Written in batches by
-- activitylog.Recorder off the request path; queried by hand (no endpoint). No body or query string is kept
-- (they can hold passwords and tokens). user_id / session_id have no FK on purpose: a log row outlives
-- anything and the insert stays cheap. NULL = not signed in.
CREATE TABLE api_activity_logs
(
    id             bigserial PRIMARY KEY,
    occurred_at    timestamptz   NOT NULL,
    request_id     varchar(128)  NOT NULL DEFAULT '',
    user_id        bigint,
    session_id     uuid,
    method         varchar(10)   NOT NULL,
    route          varchar(255)  NOT NULL DEFAULT '',
    path           varchar(2048) NOT NULL,
    status         integer       NOT NULL,
    latency_ms     integer       NOT NULL,
    response_bytes integer       NOT NULL,
    client_ip      varchar(64)   NOT NULL DEFAULT '',
    user_agent     varchar(512)  NOT NULL DEFAULT ''
);

COMMENT ON COLUMN api_activity_logs.route IS 'the matched route pattern, e.g. /api/notes/:id; empty when none matched (404 / 405)';

CREATE INDEX api_activity_logs_occurred_at_idx ON api_activity_logs (occurred_at);
CREATE INDEX api_activity_logs_user_occurred_at_idx ON api_activity_logs (user_id, occurred_at) WHERE user_id IS NOT NULL;
CREATE INDEX api_activity_logs_route_occurred_at_idx ON api_activity_logs (route, occurred_at);
