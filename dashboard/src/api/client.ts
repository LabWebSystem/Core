import type { components } from "./schema";

export type Application = components["schemas"]["Application"];
export type Operation = components["schemas"]["OperationResource"];
export type Configuration = components["schemas"]["ConfigurationResponse"];

export class ApiError extends Error {
  constructor(message: string, readonly status: number) {
    super(message);
  }
}

const requestId = () => crypto.randomUUID();

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`/api/v1${path}`, {
    ...init,
    headers: { "Content-Type": "application/json", ...init?.headers },
  });
  if (!response.ok) {
    const body = await response.json().catch(() => null) as { error?: { message?: string } } | null;
    throw new ApiError(body?.error?.message ?? "操作を完了できませんでした", response.status);
  }
  return response.json() as Promise<T>;
}

const appId = (app: Application) => app.name.replace("applications/", "");

export const api = {
  list: async () => (await request<{ applications: Application[] }>("/applications")).applications,
  get: (id: string) => request<Application>(`/applications/${encodeURIComponent(id)}`),
  configuration: (id: string) => request<Configuration>(`/applications/${encodeURIComponent(id)}/configuration`),
  create: (input: { repositoryUrl: string; ref: string; subdomain: string }) =>
    request<{ name: string }>("/applications", { method: "POST", body: JSON.stringify({ ...input, requestId: requestId() }) }),
  update: (app: Application, input: { ref?: string; subdomain?: string }) =>
    request<{ name: string }>(`/applications/${encodeURIComponent(appId(app))}`, { method: "PATCH", body: JSON.stringify({ ...input, requestId: requestId() }) }),
  action: (app: Application, action: "start" | "stop" | "sync" | "rebuild" | "register") =>
    request<{ name: string }>(`/applications/${encodeURIComponent(appId(app))}:${action}`, { method: "POST", body: JSON.stringify({ requestId: requestId() }) }),
  unregister: (app: Application) =>
    request<{ name: string }>(`/applications/${encodeURIComponent(appId(app))}`, { method: "DELETE", body: JSON.stringify({ requestId: requestId() }) }),
  purge: (app: Application) =>
    request<{ name: string }>(`/applications/${encodeURIComponent(appId(app))}:purge`, { method: "POST", body: JSON.stringify({ requestId: requestId(), confirm: true }) }),
  saveConfiguration: (app: Application, variables: Record<string, { value: string; secret: boolean }>) =>
    request<{ name: string }>(`/applications/${encodeURIComponent(appId(app))}/configuration`, { method: "PATCH", body: JSON.stringify({ variables, requestId: requestId() }) }),
  operation: (name: string) => request<Operation>(`/${name}`),
  watchOperation(name: string, onState: (operation: Operation) => void, onError: () => void) {
    const source = new EventSource(`/api/v1/${name}:watch`);
    source.onmessage = (event) => onState(JSON.parse(event.data) as Operation);
    source.onerror = () => onError();
    return () => source.close();
  },
  tailLogs(app: Application, onLine: (line: string) => void) {
    const source = new EventSource(`/api/v1/applications/${encodeURIComponent(appId(app))}:tailLogs`);
    source.onmessage = (event) => onLine(event.data);
    return () => source.close();
  },
};
