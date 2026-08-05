# Log System Design

## 1. Purpose

The Log System records NexGestion system events and user actions consistently. It writes logs to the `log` directory at the project root and provides an authenticated API through which authorized users can query records by time and status.

The Log System must meet these requirements:

- Create the log directory automatically when it does not exist.
- Include the date, 24-hour time, status, source IP address, user ID, and content in every record.
- Require callers to provide only a status and content to the log function.
- Support filtering by time range and status.
- Retain logs for no more than one week and automatically delete records older than seven days.

## 2. Storage Location

The default storage location under the NexGestion project or runtime directory is:

```text
log/
```

At startup, the server must call `os.MkdirAll("log", 0755)` to ensure the path exists. It must not overwrite or clear an existing directory.

Production deployments may specify another location through the `LOG_DIR` environment variable. When it is unset, the root-level `log` directory is used.

Logs use one JSON Lines file per day:

```text
log/
├── 2026-07-05.log
└── 2026-07-06.log
```

Only the Log System may generate filenames, based on the date. APIs and clients cannot supply filenames, which prevents path traversal.

## 3. Log Record Format

Each line contains one independent JSON record:

```json
{"timestamp":"2026-07-06 14:35:21 +08:00","status":"info","ip":"192.168.1.20","user_id":"00000000-0000-0000-0000-000000000001","content":"updated user 68cd..."}
```

Field definitions:

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `timestamp` | string | Yes | `YYYY-MM-DD HH:mm:ss ±HH:MM`, using 24-hour time and including the time-zone offset |
| `status` | string | Yes | `info`, `warning`, or `error` |
| `ip` | string | Yes | Client IP address that initiated the action; an empty string for non-HTTP background work |
| `user_id` | string | Yes | The `users.id` associated with the JWT; an empty string for unauthenticated or background work |
| `content` | string | Yes | Log content; must not contain passwords, raw tokens, or other secrets |

Timestamps use the server's configured time zone. NexGestion defaults to `Asia/Taipei`. Internal filtering must parse timestamps as time values with time-zone information instead of comparing display strings directly.

JSON Lines prevents line breaks or special characters in content from corrupting the log structure. Every write must use a JSON encoder rather than manually concatenating strings.

## 4. Status Levels

Only three statuses are accepted:

- `info`: normal operations, such as a successful login or data creation or update.
- `warning`: recoverable but notable events, such as a failed login or input validation failure.
- `error`: failed operations or system errors, such as database or file-write failures.

Status input must be normalized to lowercase. A value outside the accepted set must return an error and must not silently become `info`.

## 5. Request Context

After Authentication Middleware validates the JWT, Log Middleware creates a request-scoped logger and automatically binds:

- the client IP address; and
- the user ID from the JWT `sub` claim.

The flow is:

```text
HTTP Request
  -> Authentication Middleware
  -> Log Middleware: bind IP and user ID
  -> API Handler
  -> Log(status, content)
  -> log/YYYY-MM-DD.log
```

Clients cannot specify a log record's `user_id` in a request body or header. The user ID must come only from a verified JWT.

When the service runs behind a reverse proxy, it may read `X-Forwarded-For` only if the proxy address is in the trusted proxy list. Otherwise, it must use `RemoteAddr` to prevent clients from spoofing an IP address.

## 6. Log Function

Callers provide only the status and content:

```go
logger := logs.FromContext(r.Context())

if err := logger.Log("info", "created user "+userID); err != nil {
    // Handle log write failure.
}
```

Recommended interface:

```go
type RequestLogger interface {
    Log(status string, content string) error
}
```

The logger returned by `FromContext` already contains the IP address and user ID, so the `Log` function needs no additional parameters. Mutable global variables must not hold the current request's user or IP address because concurrent requests could contaminate one another.

Background work uses the System Logger and also provides only a status and content. Its `ip` and `user_id` values are empty strings:

```go
systemLogger.Log("error", "database backup failed")
```

Log writes must support concurrent calls from multiple goroutines. Use a mutex or a single writer queue to prevent JSON records from interleaving. A call must not report success before the write succeeds, and a log-write failure must not cause a server panic.

## 7. Events to Record

The first phase must record at least:

- successful and failed logins;
- account locks;
- successful and failed refresh-token operations;
- logouts;
- user creation, updates, and deletion;
- internal API errors; and
- server startup and shutdown.

Content for user actions should include the action and target ID, for example:

```text
created user 68cd...
updated user 68cd...
deleted user 68cd...
```

Never record:

- plaintext passwords or `password_hash`;
- access tokens or refresh tokens;
- cookies or the `Authorization` header;
- the JWT signing secret; or
- complete sensitive request bodies.

## 8. Read Log API

### 8.1 Endpoint

```http
GET /api/logs
```

This API must use Authentication Middleware. The first phase requires authentication; after the permission system is introduced, it must require the `logs.read` permission.

### 8.2 Query Parameters

| Parameter | Required | Description |
| --- | --- | --- |
| `start` | No | Start time in RFC 3339 format, for example `2026-07-06T00:00:00+08:00` |
| `end` | No | End time in RFC 3339 format, for example `2026-07-06T23:59:59+08:00` |
| `status` | No | `info`, `warning`, or `error`; a comma-separated list may select multiple statuses |
| `limit` | No | Number of records to return; default 100, maximum 1000 |
| `cursor` | No | Pagination cursor that prevents loading an entire week at once |
| `page` | No | One-based page number for UI pagination; default `1` when `cursor` is not supplied |
| `page_size` | No | Number of records per page for UI pagination; default `20`, maximum `100` when `cursor` is not supplied |
| `keyword` | No | Case-insensitive search against log content, user id, and IP address |
| `sort` | No | `timestamp` or `status`; default `timestamp` |
| `order` | No | `asc` or `desc`; default `desc` |

Example:

```http
GET /api/logs?start=2026-07-06T08:00:00%2B08:00&end=2026-07-06T18:00:00%2B08:00&status=warning,error&limit=100
Authorization: Bearer <access-token>
```

When no time is supplied, the query defaults to the last 24 hours. The permitted query range cannot begin earlier than seven days before the current time.

### 8.3 Response

```json
{
  "logs": [
    {
      "timestamp": "2026-07-06 14:35:21 +08:00",
      "status": "warning",
      "ip": "192.168.1.20",
      "user_id": "00000000-0000-0000-0000-000000000001",
      "content": "login failed"
    }
  ],
  "next_cursor": "<opaque-cursor>"
}
```

Results are ordered from newest to oldest by default. `next_cursor` is an empty string when there is no next page.

For UI screens, `GET /api/logs` follows the shared List API Query Standard in [`architecture.md`](./architecture.md) and returns the array property `logs`. Cursor pagination remains available for log streaming or export-like reads. A request must use either cursor pagination or page pagination, not both.

### 8.4 Validation and Errors

- Invalid time or status format: `400 Bad Request`.
- Missing authentication or invalid token: `401 Unauthorized`.
- Missing log-read permission: `403 Forbidden`.
- Log-read failure: `500 Internal Server Error`; the response must not expose the real file path.

The API may read only files inside the configured log directory whose names match `YYYY-MM-DD.log`.

## 9. Retention

Logs must not be retained for more than one week. Retention uses an exact seven-day cutoff:

```text
record timestamp < current time - 7 days
```

Cleanup must run:

1. immediately at server startup;
2. every hour while the server is running; and
3. as a quick check before creating a log file for a new date.

Delete a whole daily file when all its records are older than the retention cutoff. If the cutoff falls within a file's date, rewrite that file and retain only records from the last seven days to satisfy the one-week maximum.

Cleanup may delete only `YYYY-MM-DD.log` files managed by the Log System. It must not delete other files from the `log` directory. A cleanup failure must be written to the server's standard error output; do not attempt to write it through the failing Log System and create a recursive failure.

## 10. Router Registration

Register the Log API in the centralized router and apply authentication:

```go
router.HandleFunc(
    "GET /api/logs",
    requireAuth(auth, readLogs(logService)),
)
```

Middleware order must be:

```text
Authentication -> Request Logger -> API Handler
```

This order ensures that the Request Logger receives a verified user ID.

## 11. Implementation Order

1. Create the `log` package, log-record model, and safe daily-file writer.
2. Create the log directory during server startup.
3. Create request-scoped Log Middleware.
4. Connect login and User CRUD operations to the log function.
5. Implement time and status filtering and pagination.
6. Register the protected `GET /api/logs` endpoint in the router.
7. Implement startup cleanup and an hourly retention worker.
8. Add tests for concurrent writes, filtering, permissions, and seven-day cleanup.

The daily-file writer, request-scoped logger, login and User CRUD action records, filtered and paginated `GET /api/logs`, and exact seven-day retention worker are implemented. The `logs.read` Permission Middleware remains future work.
