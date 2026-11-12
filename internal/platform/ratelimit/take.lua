-- KEYS[1] = bucket key; ARGV[1] = burst; ARGV[2] = interval in microseconds (Per / Burst); ARGV[3] = ttl in ms
local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000000 + tonumber(t[2])
local burst = tonumber(ARGV[1])
local interval = tonumber(ARGV[2])
local b = redis.call('HMGET', KEYS[1], 'tokens', 'ts')
local tokens = tonumber(b[1])
local ts = tonumber(b[2])
if tokens == nil or ts == nil then
  tokens = burst
  ts = now
end
if now > ts then
  tokens = math.min(burst, tokens + (now - ts) / interval)
  ts = now
end
local allowed = 0
local wait = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
else
  wait = math.ceil((1 - tokens) * interval)
end
-- string.format, not tostring: Lua 5.1's tostring keeps 14 digits, which rounds the µs timestamp (16 digits)
-- to 100 µs; %.0f keeps it exact (whole µs, < 2^53) and %.17g every bit of the fractional tokens.
redis.call('HSET', KEYS[1], 'tokens', string.format('%.17g', tokens), 'ts', string.format('%.0f', ts))
redis.call('PEXPIRE', KEYS[1], ARGV[3])
return {allowed, wait}
