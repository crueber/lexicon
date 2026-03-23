import {
  type Component,
  type JSX,
  createContext,
  createSignal,
  onMount,
  useContext,
} from "solid-js";
import {
  api,
  ApiError,
  setAccessToken,
  setRefreshToken,
  getAccessToken,
  getRefreshToken,
  clearTokens,
} from "../../shared/api/client";

export interface User {
  id: number;
  username: string;
  email: string;
  name: string;
  role: string;
}

export interface AuthContextValue {
  user: () => User | null;
  isAuthenticated: () => boolean;
  isAdmin: () => boolean;
  login: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  loading: () => boolean;
}

const AuthContext = createContext<AuthContextValue>();

interface LoginResponse {
  accessToken: string;
  refreshToken: string;
  user: User;
}

const AuthProvider: Component<{ children: JSX.Element }> = (props) => {
  const [user, setUser] = createSignal<User | null>(null);
  const [loading, setLoading] = createSignal(true);

  const isAuthenticated = () => user() !== null;
  const isAdmin = () => user()?.role === "ADMIN";

  async function fetchMe(): Promise<User | null> {
    try {
      return await api<User>("/auth/me");
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        clearTokens();
        return null;
      }
      // Network error or other issue — treat as unauthenticated.
      return null;
    }
  }

  async function login(username: string, password: string): Promise<void> {
    const resp = await api<LoginResponse>("/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    setAccessToken(resp.accessToken);
    setRefreshToken(resp.refreshToken);
    setUser(resp.user);
  }

  async function logout(): Promise<void> {
    const refreshToken = getRefreshToken();
    if (refreshToken) {
      try {
        await fetch("/api/auth/logout", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ refreshToken }),
        });
      } catch {
        // Best-effort logout — server may be unreachable.
      }
    }
    clearTokens();
    setUser(null);
  }

  // On mount: check for stored token and validate it.
  onMount(async () => {
    const token = getAccessToken();
    if (token) {
      const me = await fetchMe();
      setUser(me);
    }
    setLoading(false);
  });

  const value: AuthContextValue = {
    user,
    isAuthenticated,
    isAdmin,
    login,
    logout,
    loading,
  };

  return (
    <AuthContext.Provider value={value}>
      {props.children}
    </AuthContext.Provider>
  );
};

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) {
    throw new Error("useAuth must be used inside AuthProvider");
  }
  return ctx;
}

export default AuthProvider;
