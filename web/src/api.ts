export type APIResponse<T> = {
  ok: boolean;
  data?: T;
  error?: string;
};

export type Host = {
  name: string;
  ip: string;
  auth_ref?: string;
  user?: string;
  password?: string;
  password_enc?: string;
  key_path?: string;
  remark?: string;
  group?: string;
  port?: number;
  jump?: string;
  backend?: string;
  via?: string;
  run_template?: string;
  login_command?: string;
};

export type AuthProfile = {
  name: string;
  user?: string;
  password?: string;
  password_enc?: string;
  key_path?: string;
  remark?: string;
};

export type ConfigSummary = {
  path: string;
  logs_path: string;
  defaults: Record<string, unknown>;
  host_count: number;
  auth_count: number;
  readonly: boolean;
  doctor: Array<{ level: string; item: string; message: string }>;
  doctor_ok: boolean;
};

export type LogRecord = {
  time?: string;
  task_id?: string;
  host?: string;
  target?: string;
  command?: string;
  status?: string;
  duration_ms?: number;
  output_inline?: boolean;
  output_bytes?: number;
  output_file?: string;
  error?: string;
  [key: string]: unknown;
};

export class APIError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}

export async function apiGet<T>(path: string): Promise<T> {
  return request<T>(path);
}

export async function apiPost<T>(path: string, body?: unknown): Promise<T> {
  return request<T>(path, { method: "POST", body: body === undefined ? undefined : JSON.stringify(body) });
}

export async function apiPut<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, { method: "PUT", body: JSON.stringify(body) });
}

export async function apiDelete<T>(path: string): Promise<T> {
  return request<T>(path, { method: "DELETE" });
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body) {
    headers.set("Content-Type", "application/json");
  }
  const res = await fetch(path, { ...init, headers });
  const payload = (await res.json()) as APIResponse<T>;
  if (!res.ok || !payload.ok) {
    throw new APIError(payload.error || res.statusText, res.status);
  }
  return payload.data as T;
}
