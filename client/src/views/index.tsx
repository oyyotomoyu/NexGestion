import { Navigate, RouteObject, useRoutes } from "react-router-dom";

import { ProtectedRoute } from "@/auth/ProtectedRoute";
import { useAuth } from "@/auth/AuthProvider";
import AppLayout from "@/layouts/AppLayout";
import Attendance from "@/views/Attendance";
import AttendanceApprovals from "@/views/AttendanceApprovals";
import Dashboard from "@/views/Dashboard";
import Login from "@/views/Login";
import Notifications from "@/views/Notifications";
import Salary from "@/views/Salary";
import SalaryEmployees from "@/views/SalaryEmployees";
import Templates from "@/views/Templates";
import Settings from "@/views/Settings";
import Roles from "@/views/Settings/Roles";
import RoleDetail from "@/views/Settings/Roles/RoleDetail";
import Groups from "@/views/Settings/Groups";
import GroupDetail from "@/views/Settings/Groups/GroupDetail";
import Users from "@/views/Settings/Users";
import UserDetail from "@/views/Settings/Users/UserDetail";

function SettingsDefaultRedirect({ nested = false }: { nested?: boolean }) {
  const { user } = useAuth();
  const hasPermission = (key: string) =>
    user?.is_protected === true || user?.roles.some((role) =>
      role.grants_all_permissions ||
      role.permissions.some((permission) => permission.permission_key === key),
    ) === true;

  const prefix = nested ? "" : "access-control/";
  if (hasPermission("roles.access")) return <Navigate to={`${prefix}roles`} replace />;
  if (hasPermission("users.access")) return <Navigate to={`${prefix}users`} replace />;
  if (hasPermission("groups.access")) return <Navigate to={`${prefix}groups`} replace />;
  return <Navigate to="/dashboard" replace />;
}

const routes: RouteObject[] = [
  {
    path: "/login",
    element: <Login />,
  },
  {
    path: "/",
    element: (
      <ProtectedRoute>
        <AppLayout />
      </ProtectedRoute>
    ),
    children: [
      {
        index: true,
        element: <Navigate to="/dashboard" replace />,
      },
      {
        path: "dashboard",
        element: <Dashboard />,
      },
      {
        path: "attendance",
        element: <ProtectedRoute permission="attendance.access"><Attendance /></ProtectedRoute>,
      },
      {
        path: "attendance/approvals",
        element: <ProtectedRoute permission="attendance.access"><AttendanceApprovals /></ProtectedRoute>,
      },
      {
        path: "notifications",
        element: <ProtectedRoute permission="notifications.access"><Notifications /></ProtectedRoute>,
      },
      {
        path: "salary",
        element: <ProtectedRoute permission="salary.access"><Salary /></ProtectedRoute>,
      },
      {
        path: "salary/employees",
        element: <ProtectedRoute permission="salary.access"><SalaryEmployees /></ProtectedRoute>,
      },
      {
        path: "templates",
        element: <ProtectedRoute permission="templates.access"><Templates /></ProtectedRoute>,
      },
      {
        path: "settings",
        element: <Settings />,
        children: [
          {
            index: true,
            element: <SettingsDefaultRedirect />,
          },
          { path: "profile/:userId", element: <UserDetail /> },
          {
            path: "access-control",
            children: [
              {
                index: true,
                element: <SettingsDefaultRedirect nested />,
              },
              {
                path: "roles",
                element: <ProtectedRoute permission="roles.access"><Roles /></ProtectedRoute>,
              },
              { path: "users", element: <ProtectedRoute permission="users.access"><Users /></ProtectedRoute> },
              { path: "users/:userId", element: <ProtectedRoute permission="users.access"><UserDetail /></ProtectedRoute> },
              {
                path: "roles/:roleId",
                element: <ProtectedRoute permission="roles.access"><RoleDetail /></ProtectedRoute>,
              },
              {
                path: "groups",
                element: <ProtectedRoute permission="groups.access"><Groups /></ProtectedRoute>,
              },
              {
                path: "groups/:groupId",
                element: <ProtectedRoute permission="groups.access"><GroupDetail /></ProtectedRoute>,
              },
            ],
          },
        ],
      },
    ],
  },
];

export default function AppRoutes() {
  return useRoutes(routes);
}
