-- name: InsertActivityLogs :exec
-- One batch of activity rows in one statement: each argument is a column, the
-- i-th elements are one row (the unnest calls run in lockstep). user_id 0 and
-- the nil session_id mean "not signed in" (NULL).
INSERT INTO api_activity_logs (occurred_at, request_id, user_id, session_id, method, route, path, status,
                               latency_ms, response_bytes, client_ip, user_agent, api_client)
SELECT r.occurred_at,
       r.request_id,
       NULLIF(r.user_id, 0),
       NULLIF(r.session_id, '00000000-0000-0000-0000-000000000000'::uuid),
       r.method,
       r.route,
       r.path,
       r.status,
       r.latency_ms,
       r.response_bytes,
       r.client_ip,
       r.user_agent,
       r.api_client
FROM (SELECT unnest(sqlc.arg(occurred_at)::timestamptz[]) AS occurred_at,
             unnest(sqlc.arg(request_id)::varchar[])      AS request_id,
             unnest(sqlc.arg(user_id)::bigint[])          AS user_id,
             unnest(sqlc.arg(session_id)::uuid[])         AS session_id,
             unnest(sqlc.arg(method)::varchar[])          AS method,
             unnest(sqlc.arg(route)::varchar[])           AS route,
             unnest(sqlc.arg(path)::varchar[])            AS path,
             unnest(sqlc.arg(status)::integer[])          AS status,
             unnest(sqlc.arg(latency_ms)::integer[])      AS latency_ms,
             unnest(sqlc.arg(response_bytes)::integer[])  AS response_bytes,
             unnest(sqlc.arg(client_ip)::varchar[])       AS client_ip,
             unnest(sqlc.arg(user_agent)::varchar[])      AS user_agent,
             unnest(sqlc.arg(api_client)::varchar[])      AS api_client) r;
