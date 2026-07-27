import { Navigate, RouteObject, useRoutes } from "react-router-dom";

import { ProtectedRoute } from "@/auth/ProtectedRoute";
import AppLayout from "@/layouts/AppLayout";
import Dashboard from "@/views/Dashboard";
import Login from "@/views/Login";
import Settings from "@/views/Settings";
import Roles from "@/views/Settings/Roles";
import RoleDetail from "@/views/Settings/Roles/RoleDetail";
import Groups from "@/views/Settings/Groups";
import GroupDetail from "@/views/Settings/Groups/GroupDetail";
import Users from "@/views/Settings/Users";
import UserDetail from "@/views/Settings/Users/UserDetail";

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
        path: "settings",
        element: <Settings />,
        children: [
          {
            index: true,
            element: <Navigate to="access-control/roles" replace />,
          },
          { path: "profile/:userId", element: <UserDetail /> },
          {
            path: "access-control",
            children: [
              {
                index: true,
                element: <Navigate to="roles" replace />,
              },
              {
                path: "roles",
                element: <Roles />,
              },
              { path: "users", element: <Users /> },
              { path: "users/:userId", element: <UserDetail /> },
              {
                path: "roles/:roleId",
                element: <RoleDetail />,
              },
              {
                path: "groups",
                element: <Groups />,
              },
              {
                path: "groups/:groupId",
                element: <GroupDetail />,
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
