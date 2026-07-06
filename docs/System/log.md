# Log System Design

## 1. Purpose

Log System 統一記錄 NexGestion 的系統事件與使用者操作。Log 寫入專案根目錄的 `log` 資料夾，並提供受登入保護的 API，讓具備權限的使用者依時間與狀態查詢紀錄。

Log System 必須符合以下需求：

- Log 資料夾不存在時自動建立。
- 每筆 Log 包含日期、24 小時制時間、狀態、來源 IP、使用者 ID 與內容。
- 呼叫 Log function 時只需要提供狀態與內容。
- 支援依時間範圍及狀態篩選。
- Log 最多保留一週，超過七天自動刪除。

## 2. Storage Location

預設儲存位置為 NexGestion 專案或執行目錄下的：

```text
log/
```

Server 啟動時必須使用 `os.MkdirAll("log", 0755)` 確保路徑存在。路徑已存在時不得覆寫或清空內容。

正式部署可透過 `LOG_DIR` 環境變數指定其他位置；未設定時使用根目錄的 `log`。

Log 採每日一個 JSON Lines 檔案：

```text
log/
├── 2026-07-05.log
└── 2026-07-06.log
```

檔名只能由 Log System 根據日期產生，不接受 API 或 Client 指定檔名，避免 path traversal。

## 3. Log Record Format

每一行代表一筆獨立 JSON：

```json
{"timestamp":"2026-07-06 14:35:21 +08:00","status":"info","ip":"192.168.1.20","user_id":"00000000-0000-0000-0000-000000000001","content":"updated user 68cd..."}
```

欄位定義：

| Field | Type | Required | Description |
| --- | --- | --- | --- |
| `timestamp` | string | Yes | `YYYY-MM-DD HH:mm:ss ±HH:MM`，使用 24 小時制並包含時區 |
| `status` | string | Yes | `info`, `warning`, `error` |
| `ip` | string | Yes | 發出操作的 Client IP；非 HTTP 背景工作使用空字串 |
| `user_id` | string | Yes | JWT 對應的 `users.id`；未登入或背景工作使用空字串 |
| `content` | string | Yes | Log 內容，不得包含密碼、原始 Token 或其他 Secret |

時間使用 Server 設定的時區；NexGestion 預設為 `Asia/Taipei`。內部篩選時應解析為含時區的時間值，避免直接以顯示字串比較。

JSON Lines 可避免 content 中的換行或特殊字元破壞 Log 結構。每次寫入必須使用 JSON encoder，不得自行拼接字串。

## 4. Status Levels

僅接受三種狀態：

- `info`：正常操作，例如登入成功、新增或更新資料。
- `warning`：可恢復但值得注意的事件，例如登入失敗、輸入驗證失敗。
- `error`：操作失敗或系統異常，例如資料庫錯誤、檔案寫入失敗。

狀態輸入應正規化為小寫。不在允許清單中的狀態必須回傳錯誤，不得靜默改成 `info`。

## 5. Request Context

Log Middleware 在 Authentication Middleware 驗證 JWT 後建立 request-scoped logger，並自動綁定：

- Client IP；
- JWT `sub` claim 中的 user ID。

流程如下：

```text
HTTP Request
  -> Authentication Middleware
  -> Log Middleware: bind IP and user ID
  -> API Handler
  -> Log(status, content)
  -> log/YYYY-MM-DD.log
```

Client 不得在 Request body 或 header 自行指定 Log 的 `user_id`。使用者 ID 只能來自已驗證的 JWT。

若服務部署在 reverse proxy 後方，只有在 proxy 位址位於受信任清單時，才可讀取 `X-Forwarded-For`。否則一律使用 `RemoteAddr`，避免 Client 偽造 IP。

## 6. Log Function

呼叫端只需要提供狀態與內容：

```go
logger := logs.FromContext(r.Context())

if err := logger.Log("info", "created user "+userID); err != nil {
    // Handle log write failure.
}
```

建議介面：

```go
type RequestLogger interface {
    Log(status string, content string) error
}
```

`FromContext` 取得的 logger 已包含 IP 與 user ID，因此 `Log` function 不需要額外參數。不得使用可變的全域變數保存目前 request 的使用者或 IP，否則多個 request 並行時可能互相污染。

背景工作使用 System Logger，同樣只傳入狀態與內容，但其 `ip` 和 `user_id` 為空字串：

```go
systemLogger.Log("error", "database backup failed")
```

Log 寫入必須支援多個 goroutine 並行呼叫，使用 mutex 或單一 writer queue 避免多筆 JSON 交錯。寫入成功前不得回報成功；Log 寫入失敗時不得造成 Server panic。

## 7. Events to Record

第一階段至少記錄：

- 登入成功與失敗；
- 帳號被鎖定；
- Refresh Token 成功或失敗；
- 登出；
- 新增、編輯、刪除使用者；
- API 發生內部錯誤；
- Server 啟動與關閉。

使用者操作的 content 應包含 action 與 target ID，例如：

```text
created user 68cd...
updated user 68cd...
deleted user 68cd...
```

禁止記錄：

- 明文密碼或 `password_hash`；
- Access Token 或 Refresh Token；
- Cookie、Authorization header；
- JWT signing secret；
- 完整的敏感 Request body。

## 8. Read Log API

### 8.1 Endpoint

```http
GET /api/logs
```

此 API 必須通過 Authentication Middleware。第一階段要求登入；加入權限系統後應要求 `logs.read` permission。

### 8.2 Query Parameters

| Parameter | Required | Description |
| --- | --- | --- |
| `start` | No | 起始時間，RFC 3339，例如 `2026-07-06T00:00:00+08:00` |
| `end` | No | 結束時間，RFC 3339，例如 `2026-07-06T23:59:59+08:00` |
| `status` | No | `info`, `warning`, `error`；可用逗號指定多個狀態 |
| `limit` | No | 回傳筆數，預設 100，最大 1000 |
| `cursor` | No | 分頁游標，避免一次載入整週 Log |

範例：

```http
GET /api/logs?start=2026-07-06T08:00:00%2B08:00&end=2026-07-06T18:00:00%2B08:00&status=warning,error&limit=100
Authorization: Bearer <access-token>
```

若未提供時間，預設查詢最近 24 小時。允許查詢的時間範圍不得早於目前時間減七天。

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

結果預設依時間由新到舊排序。沒有下一頁時 `next_cursor` 為空字串。

### 8.4 Validation and Errors

- 時間或狀態格式錯誤：`400 Bad Request`。
- 未登入或 Token 無效：`401 Unauthorized`。
- 沒有讀取 Log 權限：`403 Forbidden`。
- Log 讀取失敗：`500 Internal Server Error`，不得回傳實際檔案路徑。

API 只能讀取符合 `YYYY-MM-DD.log` 格式且位於設定 Log 目錄內的檔案。

## 9. Retention

Log 不得保存超過一週。保留規則採用精確七天：

```text
record timestamp < current time - 7 days
```

清理程序應在以下時機執行：

1. Server 啟動時立即清理一次。
2. Server 運作期間每小時檢查一次。
3. 建立新日期 Log 檔案前再次快速檢查。

每日檔案整份早於保留期限時直接刪除。如果期限落在某個檔案日期中間，應重寫該檔案，只保留七天內的紀錄，才能符合「不超過一週」。

清理時只能刪除 Log System 管理的 `YYYY-MM-DD.log`，不得刪除 `log` 目錄中的其他檔案。清理失敗需寫入 Server 的標準錯誤輸出，避免嘗試寫入同一個故障中的 Log System 而形成遞迴。

## 10. Router Registration

Log API 應在集中式 Router 中註冊並套用驗證：

```go
router.HandleFunc(
    "GET /api/logs",
    requireAuth(auth, readLogs(logService)),
)
```

Middleware 順序應為：

```text
Authentication -> Request Logger -> API Handler
```

如此可確保 Request Logger 取得的是已驗證的 user ID。

## 11. Implementation Order

1. 建立 `log` package、Log record model 與安全的每日檔案 writer。
2. Server 啟動時建立 Log 目錄。
3. 建立 request-scoped Log Middleware。
4. 將登入及 User CRUD 操作接上 Log function。
5. 實作時間、狀態篩選與分頁查詢。
6. 在 Router 註冊受保護的 `GET /api/logs`。
7. 實作啟動清理與每小時 retention worker。
8. 加入並行寫入、篩選、權限與七天清理測試。

目前已完成每日檔案 writer、request-scoped logger、登入與 User CRUD 操作紀錄、`GET /api/logs` 篩選與分頁，以及精確七天 retention worker。`logs.read` Permission Middleware 仍屬後續工作。
