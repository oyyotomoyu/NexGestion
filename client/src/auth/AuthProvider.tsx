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
import { getUser } from "@/requests/users";
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
const devUserId = import.meta.env.VITE_DEV_USER_ID ?? "0";

export function AuthProvider({ children }: { children: ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>(
    "checking",
  );
  const [user, setUser] = useState<User | null>(null);

  useEffect(() => {
    if (isDevAuthBypassed) {
      let isMounted = true;

      async function restoreDevSession() {
        try {
          const devUser = await getUser(devUserId);
          if (!isMounted) return;
          setUser(devUser);
          setStatus("authenticated");
        } catch {
          if (!isMounted) return;
          setUser(null);
          setStatus("unauthenticated");
        }
      }

      void restoreDevSession();

      return () => {
        isMounted = false;
      };
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
      const devUser = await getUser(devUserId);
      setUser(devUser);
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
      setUser(null);
      setStatus("unauthenticated");
      return;
    }

    try {
      await logout();
    } finally {
      setUser(null);
      setStatus("unauthenticated");
    }
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
