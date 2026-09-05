import type { components } from "./schema";

export type Application = components["schemas"]["Application"];
export type Operation = components["schemas"]["OperationResource"];
export type Configuration = components["schemas"]["ConfigurationResponse"];
export type LogEntry = components["schemas"]["LogEntry"];
type LogEntryList = components["schemas"]["LogEntryList"];
export type LogQuery = { limit?: number; startAt?: string; endAt?: string };
export type ResourcePools = components["schemas"]["ResourcePools"];
export type CreateApplicationInput = Omit<
  components["schemas"]["CreateApplicationRequest"],
  "requestId"
>;
type OperationEvent = {
  type: Operation["state"];
  data?: { message?: string; phase?: string };
};

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
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
    const body = (await response.json().catch(() => null)) as {
      error?: { message?: string };
    } | null;
    throw new ApiError(body?.error?.message ?? "操作を完了できませんでした", response.status);
  }
  return response.json() as Promise<T>;
}

const appId = (app: Application) => app.name.replace("applications/", "");
const operationName = (name: string) =>
  name.startsWith("operations/") ? name : `operations/${name}`;

export const api = {
  list: async () => (await request<{ applications: Application[] }>("/applications")).applications,
  get: (id: string) => request<Application>(`/applications/${encodeURIComponent(id)}`),
  configuration: (id: string) =>
    request<Configuration>(`/applications/${encodeURIComponent(id)}/configuration`),
  resourcePools: (includeSystem = false) =>
    request<ResourcePools>(`/resource-pools${includeSystem ? "?includeSystem=true" : ""}`),
  deleteResourcePoolVolume: (name: string) =>
    request<{ id: string }>(`/resource-pools/volumes/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),
  create: (input: CreateApplicationInput) =>
    request<{ name: string }>("/applications", {
      method: "POST",
      body: JSON.stringify({ ...input, requestId: requestId() }),
    }),
  update: (app: Application, input: { ref?: string; subdomain?: string }) =>
    request<{ name: string }>(`/applications/${encodeURIComponent(appId(app))}`, {
      method: "PATCH",
      body: JSON.stringify({ ...input, requestId: requestId() }),
    }),
  action: (app: Application, action: "start" | "stop" | "sync" | "rebuild" | "register") =>
    request<{ name: string }>(`/applications/${encodeURIComponent(appId(app))}:${action}`, {
      method: "POST",
      body: JSON.stringify({ requestId: requestId() }),
    }),
  unregister: (app: Application) =>
    request<{ name: string }>(`/applications/${encodeURIComponent(appId(app))}`, {
      method: "DELETE",
      body: JSON.stringify({ requestId: requestId() }),
    }),
  purge: (app: Application) =>
    request<{ name: string }>(`/applications/${encodeURIComponent(appId(app))}:purge`, {
      method: "POST",
      body: JSON.stringify({ requestId: requestId(), confirm: true }),
    }),
  saveConfiguration: (
    app: Application,
    variables: Record<string, { value?: string; secret: boolean; keep?: boolean }>,
    deviceBindings: { service: string; targetPath: string; deviceId: string }[],
    ...publicInterface: [] | [string, number]
  ) =>
    request<{ name: string }>(`/applications/${encodeURIComponent(appId(app))}/configuration`, {
      method: "PATCH",
      // 公開先は設定画面で取得できた場合だけ送信する。
      body: JSON.stringify({
        variables,
        deviceBindings,
        ...(publicInterface.length
          ? { publicService: publicInterface[0], publicPort: publicInterface[1] }
          : {}),
        requestId: requestId(),
      }),
    }),
  createPoolDevice: (name: string, candidateStableId: string) =>
    request<{ id: string }>("/resource-pools/devices", {
      method: "POST",
      body: JSON.stringify({ name, candidateStableId }),
    }),
  operation: (name: string) => request<Operation>(`/${operationName(name)}`),
  watchOperation(
    name: string,
    onState: (operation: Pick<Operation, "name" | "state" | "phase" | "displayMessage">) => void,
    onError: () => void,
  ) {
    name = operationName(name);
    const source = new EventSource(`/api/v1/${name}:watch`);
    let finished = false;
    const receive = (event: Event) => {
      const payload = JSON.parse((event as MessageEvent<string>).data) as OperationEvent;
      const operation = {
        name,
        state: payload.type,
        phase: payload.data?.phase ?? "",
        displayMessage: payload.data?.message ?? "",
      };
      onState(operation);
      if (["succeeded", "failed", "cancelled"].includes(operation.state)) {
        finished = true;
        source.close();
      }
    };
    for (const state of ["queued", "running", "succeeded", "failed", "cancelled"] as const)
      source.addEventListener(state, receive);
    source.onerror = () => {
      if (!finished) onError();
    };
    return () => {
      finished = true;
      source.close();
    };
  },
  tailLogs(
    app: Application,
    view: "task" | "application" | "related",
    service: string | undefined,
    query: LogQuery,
    onEntry: (entry: LogEntry) => void,
  ) {
    let closed = false;
    let source: EventSource | undefined;
    const params = new URLSearchParams({ view });
    if (service) params.set("service", service);
    if (query.limit) params.set("limit", String(query.limit));
    if (query.startAt) params.set("startAt", query.startAt);
    if (query.endAt) params.set("endAt", query.endAt);
    const path = `/applications/${encodeURIComponent(appId(app))}/logEntries?${params}`;
    void request<LogEntryList>(path)
      .then((snapshot) => {
        if (closed) return;
        snapshot.entries.forEach(onEntry);
        const after = snapshot.liveCursor
          ? `&after=${encodeURIComponent(snapshot.liveCursor)}`
          : "";
        const watchParams = new URLSearchParams({ view });
        if (service) watchParams.set("service", service);
        if (after) watchParams.set("after", snapshot.liveCursor);
        source = new EventSource(
          `/api/v1/applications/${encodeURIComponent(appId(app))}/logEntries:watch?${watchParams}`,
        );
        source.addEventListener("logEntry", (event) =>
          onEntry(JSON.parse((event as MessageEvent<string>).data) as LogEntry),
        );
      })
      .catch(() => undefined);
    return () => {
      closed = true;
      source?.close();
    };
  },
};
