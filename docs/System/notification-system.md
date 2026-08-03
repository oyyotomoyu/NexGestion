# Notification System

User-facing notification workflow documentation belongs in [`../UserApp/notifications.md`](../UserApp/notifications.md).

## 1. Permission Model

Notification settings and type configuration are administrator-controlled through `notifications.manage`.

Sending is permission based:

- `notifications.send.organization`: send to the whole organization.
- `notifications.send.group`: send to selected groups, even when the sender is not a member.
- `notifications.send.own_group`: send only to groups where the sender is a member.
- `notifications.type.*`: allow use of a specific notification type.

Users receive notifications when the notification audience matches their user id, role, group membership, own-group rule, or the whole organization scope.

## 2. Notification Types

Default types are stored in `notification_types`:

| Code | Meaning |
| --- | --- |
| `info` | General information or routine notice |
| `success` | Positive completion or confirmation notice |
| `warning` | Notice that needs attention |
| `important` | Important business notice |
| `urgent` | Urgent notice requiring fast action |

Each type may require a permission key, so the UI/API can prevent users from sending message types they are not allowed to use.

## 3. Message Lifecycle

A sender must provide:

- title;
- message;
- type;
- show time: `hour`, `day`, `week`, `month`, `year`, or `forever`; and
- one or more audiences.

Notification status values:

- `draft`: saved but not shown.
- `active`: visible during its show window.
- `edited`: changed after publication, so clients can detect that an older message needs refresh.
- `hidden`: unsent by the original sender; retained in the database.
- `expired`: no longer visible after `show_until`.

Timed notifications store `show_until` and `retain_until`. `retain_until` should be three months after expiry. A scheduled cleanup job can delete expired records after `retain_until`. `forever` notifications keep `show_until` and `retain_until` empty.

## 4. Database

The notification module uses `notification.db`.

- `notification_types`: allowed type catalog and type-level permission key.
- `notifications`: message body, sender, type, display window, status, and lifecycle timestamps.
- `notification_audiences`: destination scope: organization, selected group, own group, role, or user.
- `notification_deliveries`: per-user delivery/read/dismiss state.
- `notification_events`: audit trail for create, publish, edit, hide, expire, and delete actions.
- `notification_exports`: CSV/API export records for monthly admin reporting.

The database stores user, role, and group ids as text because those records live in `user.db`.

## 5. API

All routes require authentication.

| Method | Path | Permission | Description |
| --- | --- | --- | --- |
| `GET` | `/api/notifications/types` | `notifications.read` | List active/default notification types |
| `GET` | `/api/notifications` | `notifications.read` | List visible notifications for the current user |
| `GET` | `/api/notifications/admin` | `notifications.manage` | List all notification messages for administrator review |
| `POST` | `/api/notifications` | Sender/type permissions checked by service | Send a notification |
| `PATCH` | `/api/notifications/{id}` | Sender/type permissions checked by service | Edit a sent notification and mark it `edited` |
| `POST` | `/api/notifications/{id}/hide` | Original sender only | Hide/unsend a sent notification |
| `GET` | `/api/notifications/exports/{month}/csv` | `notifications.export` | Export notification records for `YYYY-MM` |

`POST` and `PATCH` validate the sender's audience permission and selected type permission from the user's effective role permissions.

## 6. Maintenance

Notification maintenance runs hourly with server startup maintenance. It:

1. changes active or edited timed notifications to `expired` when `show_until` passes; and
2. deletes expired timed notifications when `retain_until` passes.

Hidden notifications are retained for records and are not shown in user inboxes.
