import { Navigate, RouteObject, useRoutes } from "react-router-dom";

import AppLayout from "@/layouts/AppLayout";
import Dashboard from "@/views/Dashboard";
import Settings from "@/views/Settings";

const routes: RouteObject[] = [
  {
    path: "/",
    element: <AppLayout />,
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
      },
    ],
  },
];

export default function AppRoutes() {
  return useRoutes(routes);
}
