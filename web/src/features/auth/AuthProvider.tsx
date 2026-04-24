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

export interface UserPermissions {
  role: string;
  canDownload: boolean;
  canUpload: boolean;
  canEmailSend: boolean;
  canEditMetadata: boolean;
  opdsAccess: boolean;
}

export interface User {
  id: number;
  username: string;
  email: string;
  name: string;
  role: string;
  permissions?: UserPermissions;
  libraryIds?: number[];
}

export interface AuthContextValue {
  user: () => User | null;
  isAuthenticated: () => boolean;
  isAdmin: () => boolean;
  canEmailSend: () => boolean;
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
  const isAdmin = () => (user()?.permissions?.role ?? user()?.role) === "ADMIN";
  const canEmailSend = () => user()?.permissions?.canEmailSend ?? false;

  async function fetchMe(): Promise<User | null> {
    try {
      return await api<User>("/users/me");
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
    // Fetch full user info with permissions.
    const me = await fetchMe();
    setUser(me ?? resp.user);
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
    canEmailSend,
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
