const API_KEY_STORAGE = "breadbox.apiKey";

export function getApiKey(): string | null {
  return localStorage.getItem(API_KEY_STORAGE);
}

export function setApiKey(key: string): void {
  localStorage.setItem(API_KEY_STORAGE, key);
}

export function clearApiKey(): void {
  localStorage.removeItem(API_KEY_STORAGE);
}

export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message);
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const key = getApiKey();
  if (!key) throw new ApiError(401, "NO_API_KEY", "No API key configured");

  const res = await fetch(path, {
    ...init,
    headers: {
      "X-API-Key": key,
      "Content-Type": "application/json",
      ...(init.headers ?? {}),
    },
  });

  if (!res.ok) {
    let code = "HTTP_ERROR";
    let message = res.statusText;
    try {
      const body = await res.json();
      code = body?.error?.code ?? code;
      message = body?.error?.message ?? message;
    } catch {
      // ignore parse errors
    }
    throw new ApiError(res.status, code, message);
  }

  return res.json() as Promise<T>;
}

export interface Account {
  id: string;
  short_id: string;
  name: string;
  official_name?: string | null;
  type: string;
  subtype?: string | null;
  current_balance?: string | null;
  available_balance?: string | null;
  iso_currency_code?: string | null;
  mask?: string | null;
  is_dependent_linked: boolean;
}

export interface Category {
  id: string;
  short_id: string;
  name: string;
  parent_id?: string | null;
  color?: string | null;
  icon?: string | null;
  is_system: boolean;
  is_archived: boolean;
}
