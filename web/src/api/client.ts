export class ApiError extends Error {
  constructor(public status: number, public code: string, message: string) {
    super(message);
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  // Default Content-Type is JSON, but skip it when the body is a FormData
  // (multipart) — the browser MUST set Content-Type itself so it can include
  // the auto-generated multipart boundary. Forcing application/json here
  // would leave the server unable to parse the body.
  const isFormData =
    typeof FormData !== "undefined" && init.body instanceof FormData;
  const baseHeaders: Record<string, string> = isFormData
    ? {}
    : { "Content-Type": "application/json" };
  const res = await fetch(path, {
    credentials: "include",
    ...init,
    headers: {
      ...baseHeaders,
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

  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}
