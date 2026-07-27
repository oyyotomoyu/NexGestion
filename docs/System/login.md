# Login Design

## 1. Purpose

All NexGestion business APIs must require authentication. Only essential endpoints such as health checks, login, and access-token refresh may allow anonymous access.

The login system follows the layered design used by xinsight:

```text
HTTP Request
  -> Router
  -> Authentication Middleware
  -> API Handler
  -> System Service
  -> user.db
```

The router determines whether an endpoint requires authentication. Middleware validates tokens consistently. API handlers manage HTTP and JSON concerns. System services must perform all account and session data operations.

## 2. Authentication Flow

### 2.1 Login

1. The client submits an email and password.
2. The system trims surrounding whitespace from the email and converts it to lowercase.
3. The system finds a non-deleted user and checks the account status and lock expiration.
4. The system compares the password against `password_hash` using bcrypt. It must never decrypt or return the password hash.
5. After a successful login, the system clears the failed-attempt count, updates `last_login_at`, and issues an access token and refresh token.
6. The client includes the access token in the `Authorization` header of subsequent API requests.

```http
Authorization: Bearer <access-token>
```

### 2.2 Authenticated Request

Authentication Middleware must perform these checks in order:

1. The `Authorization` header exists and uses the `Bearer` scheme.
2. The access token has a valid signature, purpose, and expiration time.
3. The user referenced by the token still exists and has not been soft-deleted.
4. The user's `status` is `active`.
5. After successful validation, the middleware places the `user_id` and token claims in the request context and passes control to the API handler.

Failed authentication returns `401 Unauthorized`. An authenticated request without sufficient permission returns `403 Forbidden`.

### 2.3 Refresh

After an access token expires, the client uses a refresh token to obtain a new access token. The system must verify that the refresh token:

- exists in `sessions`;
- has not expired;
- has not been revoked; and
- belongs to a valid, active user.

Refresh tokens must rotate after every use. The system revokes the old token and creates a new token to reduce token-reuse risk.

### 2.4 Logout

Logout revokes the current session's refresh token and clears its cookie. Access tokens do not need to be stored in the database and expire naturally after a short period.

Disabling or deleting a user, or changing a user's password, must revoke all of that user's sessions.

## 3. Token Design

### 3.1 Access Token

- Format: JWT
- Signature: HS256 with a cryptographically random secret of at least 32 bytes
- Recommended lifetime: 10 minutes
- Storage: client memory, not local storage
- Purpose: calling protected APIs

Recommended claims:

| Claim | Description |
| --- | --- |
| `sub` | Immutable `users.id` |
| `jti` | Unique token identifier |
| `iat` | Issued-at time |
| `exp` | Expiration time |
| `typ` | Always `access` |

Roles and permissions must not be embedded in a token for long-term use. Middleware or Permission Middleware must obtain current effective permissions from UserSystem so old tokens do not retain access after role changes.

### 3.2 Refresh Token

- Format: opaque token generated with cryptographically secure randomness
- Recommended lifetime: 30 days
- Storage: `HttpOnly`, `SameSite=Strict` cookie; production HTTPS environments must also use `Secure`
- Server storage: SHA-256 hash only, never the original token
- Purpose: obtaining a new access token; it cannot call business APIs directly

The JWT secret must not be hard-coded or committed to version control. It may be read from an environment variable or generated during first-time initialization and stored in a file readable only by the service process.

## 4. API Endpoints

### 4.1 Public Endpoints

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/health` | Health check |
| `POST` | `/api/auth/login` | Log in with email and password |
| `POST` | `/api/auth/refresh` | Refresh an access token with a refresh token |

Login request:

```json
{
  "email": "admin@nexgestion.local",
  "password": "user-password"
}
```

Successful login response:

```json
{
  "access_token": "<jwt>",
  "token_type": "Bearer",
  "expires_in": 600
}
```

The refresh token is returned through `Set-Cookie`, not in the JSON body.

### 4.2 Protected Endpoints

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/auth/me` | Get the currently authenticated user |
| `POST` | `/api/auth/logout` | Log out the current session |
| `GET` | `/api/users` | List all users |
| `POST` | `/api/users` | Create a user |
| `GET` | `/api/users/{id}` | Get a specific user |
| `PUT/PATCH` | `/api/users/{id}` | Edit a user |
| `DELETE` | `/api/users/{id}` | Soft-delete a user |
| `GET` | `/api/roles` | List all roles |
| `GET` | `/api/roles/{id}` | Get a role by ID |
| `POST` | `/api/roles` | Create a custom role |
| `PATCH` | `/api/roles/{id}` | Edit a custom role |
| `DELETE` | `/api/roles/{id}` | Delete an unassigned custom role |

New APIs must be placed in the protected router group by default. Every public endpoint must be registered explicitly.

## 5. Session Storage

Add `sessions` to `user.db`:

```sql
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    refresh_token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_used_at TEXT,
    revoked_at TEXT,
    ip_address TEXT,
    user_agent TEXT
);

CREATE INDEX IF NOT EXISTS sessions_user_id_idx ON sessions(user_id);
CREATE INDEX IF NOT EXISTS sessions_expires_at_idx ON sessions(expires_at);
```

A scheduled cleanup job must remove expired or revoked sessions. Each user may have at most 10 active sessions; when this limit is exceeded, the oldest session must be revoked.

## 6. Login Protection

- Email is the only login identifier in the first version and is compared case-insensitively.
- A session may be created only after successful bcrypt verification.
- After five consecutive failed login attempts, set `locked_until` to 15 minutes after the current time.
- Login is prohibited during the lock period, even with the correct password.
- A successful login resets `failed_login_count` and clears `locked_until`.
- Use the uniform error message `invalid email or password` without revealing whether an account exists.
- Apply rate limiting to login and refresh endpoints by IP address and account.
- Record successful login, failed login, lock, refresh, and logout events, but never record passwords or raw tokens.

## 7. Authorization

Authentication establishes who the user is. Authorization determines what that user may do. These concerns must remain separate.

The first phase may require only authentication for business APIs. The next phase adds Permission Middleware. Every protected operation must then check both authentication and the corresponding permission:

- `users.read`: view users
- `users.manage`: create, edit, and delete users
- `roles.read`: view roles and their permissions
- `roles.manage`: create, edit, and delete custom roles
- `roles.assign`: assign or remove user roles
- `groups.read`: view groups and members
- `groups.manage`: create, edit, and delete groups
- `groups.assign`: add or remove group members
- `permissions.read`: view permission definitions and assignments
- `permissions.assign`: assign existing permissions to custom roles; assignment is reserved for the protected initial Administrator
- `logs.read`: view request and audit logs

The initial `Admin` role has `grants_all_permissions = 1` and passes every current and future permission check. It is a protected default role that can belong only to the initial administrator; it cannot be assigned to another user, renamed, edited, deleted, or stripped of permissions. The initial administrator must permanently retain this role and capability and cannot be disabled or deleted. Authentication alone must not allow ordinary users to manage other accounts.

Only the initial Administrator may change the permissions granted by a role. A delegated role manager may still read assigned permissions. Permission definitions come from `config/permission.json`, and every protected route declares one of those keys. Every role and permission-assignment change must be recorded in the audit log.

## 8. Response Rules

| Status | Meaning |
| --- | --- |
| `200 OK` | Successful login, refresh, or read |
| `204 No Content` | Successful logout |
| `400 Bad Request` | Invalid JSON or input format |
| `401 Unauthorized` | Missing authentication, invalid token, or invalid credentials |
| `403 Forbidden` | Disabled or locked account, or insufficient permission |
| `429 Too Many Requests` | Too many login attempts |

Authentication responses must use a consistent JSON format and must not expose internal SQL, JWT, or bcrypt errors to the client.

## 9. Implementation Order

1. Create the `sessions` schema and Session System Service.
2. Implement login, refresh, logout, and current-user APIs.
3. Create Authentication Middleware.
4. Separate public and protected APIs in the router.
5. Add failed-attempt tracking, account locking, and session revocation.
6. Add Permission Middleware and login audit records.

Access tokens, refresh tokens, session rotation, login locking, and Router Authentication Middleware are implemented. User CRUD APIs require authentication. Permission Middleware, rate limiting, and login audit records remain future work.
