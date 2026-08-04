# API Test Case Matrix

This matrix defines the intended API script coverage. A case can be implemented in a module script when the matching API exists and the setup data can be created through public or protected API calls.

## Auth And Users

| Case | Level | Expected |
| --- | --- | --- |
| Public health check | 1 | `GET /api/health` returns `200` without auth |
| Login success | 1 | Admin credentials return access token and refresh cookie |
| Current user | 1 | `GET /api/auth/me` returns authenticated admin |
| User CRUD happy path | 1 | Create, list, read, update, soft-delete normal user |
| Missing auth | 2 | Protected user endpoints return `401` |
| Invalid JSON/input | 2 | Create/update return `400` |
| Duplicate email | 2 | Create returns `409` case-insensitively |
| Unknown user | 2 | Read/update/delete return `404` |
| Protected admin delete | 2 | Delete initial administrator returns `403` |
| Bad login | 2 | Invalid credentials return `401` |
| User isolation | 3 | Updating/deleting one non-protected user does not change another |
| Session lifecycle | 3 | Refresh rotates token; logout revokes refresh token |
| Account status | 3 | Disabled/locked users cannot authenticate or call protected APIs |

## Roles

| Case | Level | Expected |
| --- | --- | --- |
| Role CRUD happy path | 1 | Create, list, read, update, delete custom role |
| Missing auth/permission | 2 | Protected endpoints return `401` or `403` |
| Invalid title | 2 | Empty title returns `400` |
| Duplicate title | 2 | Duplicate title returns `409` case-insensitively |
| Unknown role | 2 | Read/update/delete returns `404` |
| System role mutation | 2 | Admin/generated group roles cannot be edited or deleted |
| Assignment lifecycle | 3 | Assign/remove custom role from a non-protected user |
| Permission grant lifecycle | 3 | Initial administrator grants/revokes catalog permission |
| Delegated manager boundary | 3 | Non-initial admin cannot grant/revoke permissions |

## Groups

| Case | Level | Expected |
| --- | --- | --- |
| Group CRUD happy path | 1 | Create, list, read, update, delete leaf group |
| Missing auth/permission | 2 | Protected endpoints return `401` or `403` |
| Invalid group input | 2 | Missing name/type/status/level returns `400` |
| Duplicate group name | 2 | Duplicate name returns `409` |
| Unknown group | 2 | Read/update/delete returns `404` |
| Child deletion policy | 2 | Parent with child group returns `409` on delete |
| Generated role policy | 3 | Group roles cannot be deleted through Role API |
| Membership lifecycle | 3 | Add/update/remove manager/member through API |
| Owner-driven deletion | 3 | Deleting leaf group removes memberships and generated roles |
| Hierarchy cycle | 3 | Parent update creating a cycle returns `400` |

## Attendance

| Case | Level | Expected |
| --- | --- | --- |
| Today/read basics | 1 | Read today, days, leave types |
| Clock workflow | 1 | Sign in and sign out in documented order |
| Invalid clock order | 2 | Repeated sign-in/sign-out errors match document |
| Leave request basics | 2 | Create/list leave request with valid type |
| Invalid leave request | 2 | Bad dates/type return documented errors |
| Approval routing | 3 | Manager/admin approval flow follows group hierarchy rules |
| Monthly reports | 3 | Generate, list, and download monthly CSV through API |
| Correction workflow | 3 | Authorized correction changes a day and records audit/report effect |

## Notifications

| Case | Level | Expected |
| --- | --- | --- |
| Type/list basics | 1 | List notification types and visible notifications |
| Send/edit/hide own notification | 1 | Sender can create, edit, and hide allowed notification |
| Missing auth | 2 | Protected endpoints reject unauthenticated requests |
| Invalid payload | 2 | Bad type/content/time returns documented errors |
| Admin list | 2 | `notifications.manage` user can list all messages |
| Sender boundary | 3 | Non-sender cannot edit/hide another sender's notification |
| Expiry/export | 3 | Timed notifications expire and monthly CSV export works |

## Logs And Report Files

| Case | Level | Expected |
| --- | --- | --- |
| Log list basics | 1 | Authorized user can query logs |
| Log filters | 2 | Date/status/limit filters match document |
| Bad log filters | 2 | Invalid date/status/limit returns `400` |
| Report file list | 1 | Authorized user can list report files |
| Report file download/delete | 2 | Existing report file can be downloaded/deleted |
| Path traversal | 2 | Traversal attempts are rejected and cannot leave report root |
| Retention/cleanup evidence | 3 | Generated reports and logs obey documented retention boundaries |
