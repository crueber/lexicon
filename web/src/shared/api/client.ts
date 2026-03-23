// Typed fetch wrapper with auth header injection and token refresh.

const ACCESS_TOKEN_KEY = "lexicon_access_token";
const REFRESH_TOKEN_KEY = "lexicon_refresh_token";

export class ApiError extends Error {
  status: number;
  body: string;

  constructor(response: Response, body?: string) {
    super(`API error: ${response.status} ${response.statusText}`);
    this.name = "ApiError";
    this.status = response.status;
    this.body = body ?? "";
  }
}

export function getAccessToken(): string | null {
  return localStorage.getItem(ACCESS_TOKEN_KEY);
}

export function setAccessToken(token: string): void {
  localStorage.setItem(ACCESS_TOKEN_KEY, token);
}

export function getRefreshToken(): string | null {
  return localStorage.getItem(REFRESH_TOKEN_KEY);
}

export function setRefreshToken(token: string): void {
  localStorage.setItem(REFRESH_TOKEN_KEY, token);
}

export function clearTokens(): void {
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
}

// Tracks whether a refresh is already in progress to avoid concurrent refreshes.
let refreshPromise: Promise<boolean> | null = null;

async function tryRefreshToken(): Promise<boolean> {
  // If a refresh is already in progress, wait for it.
  if (refreshPromise) {
    return refreshPromise;
  }

  const refreshToken = getRefreshToken();
  if (!refreshToken) {
    return false;
  }

  refreshPromise = (async () => {
    try {
      const response = await fetch("/api/auth/refresh", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ refreshToken }),
      });

      if (!response.ok) {
        clearTokens();
        return false;
      }

      const data = (await response.json()) as {
        accessToken: string;
        refreshToken: string;
      };
      setAccessToken(data.accessToken);
      setRefreshToken(data.refreshToken);
      return true;
    } catch {
      clearTokens();
      return false;
    } finally {
      refreshPromise = null;
    }
  })();

  return refreshPromise;
}

export async function api<T>(
  path: string,
  options?: RequestInit,
): Promise<T> {
  const accessToken = getAccessToken();
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(accessToken ? { Authorization: `Bearer ${accessToken}` } : {}),
    ...(options?.headers as Record<string, string> | undefined),
  };

  const response = await fetch(`/api${path}`, {
    ...options,
    headers,
  });

  if (response.status === 401) {
    // Try to refresh the token.
    const refreshed = await tryRefreshToken();
    if (refreshed) {
      // Retry the original request with the new token.
      return api<T>(path, options);
    }
    // Refresh failed — throw so the auth context can redirect to login.
    const body = await response.text().catch(() => "");
    throw new ApiError(response, body);
  }

  if (!response.ok) {
    const body = await response.text().catch(() => "");
    throw new ApiError(response, body);
  }

  return response.json() as Promise<T>;
}
