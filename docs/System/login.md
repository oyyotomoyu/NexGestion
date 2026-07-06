# Login Design

## 1. Purpose

NexGestion 的所有業務 API 都必須先完成登入驗證。只有健康檢查、登入與更新 Access Token 等必要端點可以匿名存取。

登入機制參考 xinsight 的設計，採用以下分層：

```text
HTTP Request
  -> Router
  -> Authentication Middleware
  -> API Handler
  -> System Service
  -> user.db
```

Router 決定端點是否需要登入；Middleware 統一驗證 Token；API Handler 負責 HTTP 與 JSON；帳號及 Session 的資料操作必須由 System Service 執行。

## 2. Authentication Flow

### 2.1 Login

1. Client 送出 email 與 password。
2. System 將 email 去除前後空白並轉成小寫。
3. 查詢未刪除的使用者，檢查帳號狀態與鎖定時間。
4. 使用 bcrypt 比對 `password_hash`，不得解密或回傳密碼雜湊。
5. 登入成功後清除失敗次數、更新 `last_login_at`，並簽發 Access Token 與 Refresh Token。
6. Client 將 Access Token 放在後續 API 的 `Authorization` header。

```http
Authorization: Bearer <access-token>
```

### 2.2 Authenticated Request

Authentication Middleware 必須依序檢查：

1. `Authorization` header 是否存在且使用 `Bearer` 格式。
2. Access Token 的簽章、用途與到期時間是否有效。
3. Token 對應的使用者是否仍存在且未被 soft delete。
4. 使用者 `status` 是否為 `active`。
5. 驗證成功後，將 `user_id` 與 Token claims 放入 request context，再交給 API Handler。

驗證失敗回傳 `401 Unauthorized`。已登入但沒有操作權限時回傳 `403 Forbidden`。

### 2.3 Refresh

Access Token 過期後，Client 使用 Refresh Token 取得新的 Access Token。System 必須驗證 Refresh Token 是否：

- 存在於 `sessions`；
- 尚未到期；
- 尚未撤銷；
- 對應到有效且啟用中的使用者。

Refresh Token 每次使用後應旋轉：撤銷舊 Token 並產生新 Token，降低 Token 被重複利用的風險。

### 2.4 Logout

登出時撤銷目前 Session 的 Refresh Token 並清除 Cookie。Access Token 不需要寫入資料庫，會在短時間內自然到期。

停用、刪除使用者或修改密碼時，必須撤銷該使用者的全部 Session。

## 3. Token Design

### 3.1 Access Token

- 格式：JWT
- 簽章：HS256，使用至少 32 bytes 的隨機 secret
- 建議效期：10 分鐘
- 儲存位置：Client 記憶體，不寫入 local storage
- 用途：呼叫受保護 API

建議 claims：

| Claim | Description |
| --- | --- |
| `sub` | Immutable `users.id` |
| `jti` | Token unique identifier |
| `iat` | Issued-at time |
| `exp` | Expiration time |
| `typ` | 固定為 `access` |

角色與權限不應長期固化在 Token。Middleware 或 Permission Middleware 應從 UserSystem 取得目前有效權限，避免角色異動後舊 Token 仍保有權限。

### 3.2 Refresh Token

- 格式：使用加密安全亂數產生的 opaque token
- 建議效期：30 天
- 儲存位置：`HttpOnly`, `SameSite=Strict` Cookie；正式 HTTPS 環境必須加上 `Secure`
- Server 端只儲存 SHA-256 hash，不儲存原始 Token
- 用途：取得新的 Access Token，不得直接呼叫業務 API

JWT secret 不得寫死在原始碼或提交至版本控制。可從環境變數讀取，或在首次初始化時產生並存放於只有服務程序可讀取的檔案。

## 4. API Endpoints

### 4.1 Public Endpoints

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/health` | 健康檢查 |
| `POST` | `/api/auth/login` | 使用 email/password 登入 |
| `POST` | `/api/auth/refresh` | 使用 Refresh Token 更新 Token |

登入 Request：

```json
{
  "email": "admin@nexgestion.local",
  "password": "user-password"
}
```

登入成功 Response：

```json
{
  "access_token": "<jwt>",
  "token_type": "Bearer",
  "expires_in": 600
}
```

Refresh Token 由 `Set-Cookie` 回傳，不放入 JSON body。

### 4.2 Protected Endpoints

| Method | Path | Description |
| --- | --- | --- |
| `GET` | `/api/auth/me` | 取得目前登入者資訊 |
| `POST` | `/api/auth/logout` | 登出目前 Session |
| `GET` | `/api/users` | 讀取所有使用者 |
| `POST` | `/api/users` | 新增使用者 |
| `GET` | `/api/users/{id}` | 讀取特定使用者 |
| `PUT/PATCH` | `/api/users/{id}` | 編輯使用者 |
| `DELETE` | `/api/users/{id}` | Soft delete 使用者 |

未來新增的 API 預設都應放在受保護的 Router group；公開端點必須明確逐一註冊。

## 5. Session Storage

在 `user.db` 新增 `sessions`：

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

過期或已撤銷的 Session 應由定期清理工作移除。每位使用者可限制最多 10 個有效 Session；超過時撤銷最舊的 Session。

## 6. Login Protection

- Email 是第一版唯一登入識別欄位，採不區分大小寫比對。
- bcrypt 驗證成功後才能建立 Session。
- 連續登入失敗 5 次時，設定 `locked_until` 為目前時間加 15 分鐘。
- 鎖定期間不得登入，即使密碼正確亦然。
- 登入成功後將 `failed_login_count` 歸零並清除 `locked_until`。
- 錯誤訊息統一為「email 或密碼錯誤」，不得透露帳號是否存在。
- 登入與 Refresh 端點應依 IP 與帳號做 rate limiting。
- 記錄登入成功、登入失敗、鎖定、Refresh 與登出事件，但不得記錄密碼或原始 Token。

## 7. Authorization

登入驗證只確認「使用者是誰」，權限驗證決定「使用者可以做什麼」。兩者必須分開處理。

第一階段可先要求所有業務 API 必須登入。下一階段加入 Permission Middleware：

- `users.read`：讀取使用者
- `users.manage`：新增、編輯及刪除使用者

初始 `Administrator` role 的 `grants_all_permissions = 1`，可通過所有權限檢查。一般已登入使用者不應因為成功登入就能管理其他帳號。

## 8. Response Rules

| Status | Meaning |
| --- | --- |
| `200 OK` | 登入、Refresh 或讀取成功 |
| `204 No Content` | 登出成功 |
| `400 Bad Request` | JSON 或輸入格式錯誤 |
| `401 Unauthorized` | 未登入、Token 無效或帳密錯誤 |
| `403 Forbidden` | 已登入但帳號停用、鎖定或權限不足 |
| `429 Too Many Requests` | 登入嘗試過於頻繁 |

Authentication response 應維持一致的 JSON 格式，且不得將內部 SQL、JWT 或 bcrypt 錯誤直接回傳給 Client。

## 9. Implementation Order

1. 建立 `sessions` schema 與 Session System Service。
2. 實作登入、Refresh、登出與目前使用者 API。
3. 建立 Authentication Middleware。
4. 在 Router 中區分公開及受保護 API。
5. 加入失敗次數、帳號鎖定與 Session 撤銷。
6. 加入 Permission Middleware 與登入稽核紀錄。

目前已完成 Access Token、Refresh Token、Session rotation、登入鎖定及 Router Authentication Middleware；User CRUD API 已受登入保護。Permission Middleware、rate limiting 與登入稽核紀錄仍屬後續工作。
