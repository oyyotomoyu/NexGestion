import { Navigate, useLocation } from "react-router-dom";
import type { ReactNode } from "react";

import { useAuth } from "@/auth/AuthProvider";

function hasPermission(user: ReturnType<typeof useAuth>["user"], permission: string) {
  return user?.is_protected === true || user?.roles.some((role) =>
    role.grants_all_permissions ||
    role.permissions.some((item) => item.permission_key === permission),
  ) === true;
}

export function ProtectedRoute({ children, permission }: { children: ReactNode; permission?: string }) {
  const location = useLocation();
  const { isAuthenticated, isChecking, user } = useAuth();

  if (isChecking) {
    return null;
  }

  if (!isAuthenticated) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  if (permission && !hasPermission(user, permission)) {
    return <Navigate to="/dashboard" replace />;
  }

  return <>{children}</>;
}
