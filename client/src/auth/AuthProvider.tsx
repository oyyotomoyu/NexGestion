import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";

import {
  login,
  logout,
  refreshSession,
  getCurrentUser,
  type LoginInput,
} from "@/requests/auth";
import type { User } from "@/requests/users/types";

type AuthStatus = "checking" | "authenticated" | "unauthenticated";

interface AuthContextValue {
  status: AuthStatus;
  user: User | null;
  isAuthenticated: boolean;
  isChecking: boolean;
  signIn: (input: LoginInput) => Promise<void>;
  signOut: () => Promise<void>;
}

const AuthContext = createContext<AuthContextValue | null>(null);
const isDevAuthBypassed = import.meta.env.DEV;

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>(
    isDevAuthBypassed ? "authenticated" : "checking",
  );
  const [user, setUser] = useState<User | null>(null);

  useEffect(() => {
    if (isDevAuthBypassed) {
      setStatus("authenticated");
      return;
    }

    let isMounted = true;

    async function restoreSession() {
      try {
        await refreshSession();
        const currentUser = await getCurrentUser();
        if (!isMounted) return;
        setUser(currentUser);
        setStatus("authenticated");
      } catch {
        if (!isMounted) return;
        setUser(null);
        setStatus("unauthenticated");
      }
    }

    void restoreSession();

    return () => {
      isMounted = false;
    };
  }, []);

  async function signIn(input: LoginInput) {
    if (isDevAuthBypassed) {
      setStatus("authenticated");
      return;
    }

    await login(input);
    const currentUser = await getCurrentUser();
    setUser(currentUser);
    setStatus("authenticated");
  }

  async function signOut() {
    if (isDevAuthBypassed) {
      setStatus("authenticated");
      return;
    }

    await logout();
    setUser(null);
    setStatus("unauthenticated");
  }

  const value = useMemo<AuthContextValue>(
    () => ({
      status,
      user,
      isAuthenticated: status === "authenticated",
      isChecking: status === "checking",
      signIn,
      signOut,
    }),
    [status, user],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used inside AuthProvider");
  }
  return context;
}
