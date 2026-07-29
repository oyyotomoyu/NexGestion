package apis

import (
	"net/http"

	applogs "nexgestion/server/logs"
	"nexgestion/server/system"
)

// InitRouter registers every API endpoint in one place.
//
// API handlers belong in this package and may call the system package to
// perform application operations. The router itself is only responsible for
// directing requests to the correct handler.
func InitRouter(router *http.ServeMux, users *system.UserService, attendance *system.AttendanceService, auth *system.AuthService, logService *applogs.Service) {
	catalog, err := system.LoadPermissionCatalog()
	if err != nil {
		panic("load permission catalog: " + err.Error())
	}
	// Public endpoints.
	router.HandleFunc("GET /api/health", Health)
	router.HandleFunc("POST /api/auth/login", login(auth, logService))
	router.HandleFunc("POST /api/auth/refresh", refresh(auth, logService))

	// Every protected route declares a permission from config/permission.json.
	// Adding a route without synchronizing the catalog fails during startup.
	protected := func(permission string, handler http.HandlerFunc) http.HandlerFunc {
		if !catalog.Contains(permission) {
			panic("API route uses undefined permission: " + permission)
		}
		return requireAuth(auth, withRequestLogger(logService, requirePermission(users, permission, handler)))
	}
	authenticated := func(handler http.HandlerFunc) http.HandlerFunc {
		return requireAuth(auth, withRequestLogger(logService, handler))
	}
	router.HandleFunc("GET /api/auth/me", protected("users.read", me(users)))
	router.HandleFunc("POST /api/auth/logout", protected("users.read", logout(auth)))
	router.HandleFunc("GET /api/users", protected("users.read", listUsers(users)))
	router.HandleFunc("POST /api/users", protected("users.manage", createUser(users)))
	router.HandleFunc("GET /api/users/{id}", protected("users.read", getUser(users)))
	router.HandleFunc("PUT /api/users/{id}", protected("users.manage", updateUser(users)))
	router.HandleFunc("PATCH /api/users/{id}", protected("users.manage", updateUser(users)))
	router.HandleFunc("DELETE /api/users/{id}", protected("users.manage", deleteUser(users)))
	router.HandleFunc("GET /api/roles", protected("roles.read", listRoles(users)))
	router.HandleFunc("GET /api/roles/{id}", protected("roles.read", getRole(users)))
	router.HandleFunc("POST /api/roles", protected("roles.manage", createRole(users)))
	router.HandleFunc("PATCH /api/roles/{id}", protected("roles.manage", updateRole(users)))
	router.HandleFunc("DELETE /api/roles/{id}", protected("roles.manage", deleteRole(users)))
	router.HandleFunc("GET /api/roles/{id}/users", protected("roles.read", listRoleUsers(users)))
	router.HandleFunc("PUT /api/roles/{id}/users/{userId}", protected("roles.assign", setRoleUser(users, true)))
	router.HandleFunc("DELETE /api/roles/{id}/users/{userId}", protected("roles.assign", setRoleUser(users, false)))
	router.HandleFunc("PUT /api/roles/{id}/permissions/{permissionId}", protected("permissions.assign", setRolePermission(users, true)))
	router.HandleFunc("DELETE /api/roles/{id}/permissions/{permissionId}", protected("permissions.assign", setRolePermission(users, false)))
	router.HandleFunc("GET /api/groups", protected("groups.read", listGroups(users)))
	router.HandleFunc("GET /api/groups/{id}", protected("groups.read", getGroup(users)))
	router.HandleFunc("POST /api/groups", protected("groups.manage", createGroup(users)))
	router.HandleFunc("PATCH /api/groups/{id}", protected("groups.manage", updateGroup(users)))
	router.HandleFunc("DELETE /api/groups/{id}", protected("groups.manage", deleteGroup(users)))
	router.HandleFunc("GET /api/groups/{id}/members", protected("groups.read", listGroupMembers(users)))
	router.HandleFunc("PUT /api/groups/{id}/members/{userId}", protected("groups.assign", setGroupMember(users)))
	router.HandleFunc("DELETE /api/groups/{id}/members/{userId}", protected("groups.assign", removeGroupMember(users)))
	router.HandleFunc("GET /api/permissions", protected("permissions.read", listPermissions(users)))
	router.HandleFunc("GET /api/logs", protected("logs.read", readLogs(logService)))
	router.HandleFunc("GET /api/attendance/today", protected("attendance.read.self", attendanceToday(attendance)))
	router.HandleFunc("POST /api/attendance/today/sign-in", protected("attendance.clock.self", attendanceSignIn(attendance)))
	router.HandleFunc("POST /api/attendance/today/sign-out", protected("attendance.clock.self", attendanceSignOut(attendance)))
	router.HandleFunc("GET /api/attendance/days", protected("attendance.read.self", attendanceDays(attendance, true)))
	router.HandleFunc("GET /api/attendance/leave-types", protected("attendance.read.self", attendanceLeaveTypes()))
	router.HandleFunc("GET /api/attendance/leave-requests", protected("attendance.read.self", attendanceLeaveRequests(attendance)))
	router.HandleFunc("POST /api/attendance/leave-requests", protected("attendance.clock.self", applyAttendanceLeave(attendance)))
	router.HandleFunc("GET /api/attendance/leave-approvals", authenticated(attendanceLeaveApprovals(attendance)))
	router.HandleFunc("PATCH /api/attendance/leave-approvals/{id}", authenticated(decideAttendanceLeave(attendance)))
	router.HandleFunc("GET /api/attendance/monthly/{month}", protected("attendance.read.self", attendanceSelfMonthly(attendance)))
	router.HandleFunc("GET /api/attendance/users/{userId}/days", protected("attendance.read", attendanceDays(attendance, false)))
	router.HandleFunc("GET /api/attendance/reports/{month}", protected("attendance.reports.read", attendanceMonthlyReports(attendance)))
	router.HandleFunc("POST /api/attendance/reports/{month}/generate", protected("attendance.manage", generateAttendanceMonthlyReport(attendance)))
	router.HandleFunc("GET /api/attendance/reports/{month}/csv", protected("attendance.reports.read", downloadAttendanceCSV(attendance)))
	router.HandleFunc("PATCH /api/attendance/days/{id}", protected("attendance.manage", correctAttendanceDay(attendance)))

	// Keep unknown API paths inside the API layer instead of falling through to
	// the SPA handler.
	router.HandleFunc("/api/", NotFound)
}
