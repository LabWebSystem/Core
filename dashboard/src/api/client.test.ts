import { afterEach, describe, expect, it, vi } from "vitest";
import { api, ApiError } from "./client";

describe("Dashboard API client", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("登録要求に UUID の requestId を付ける", async () => {
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ name: "operations/one" }), { status: 202 }));
    vi.stubGlobal("fetch", fetchMock);
    await api.create({ repositoryUrl: "https://github.com/example/app", ref: "main", subdomain: "test" });
    const [, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    const body = JSON.parse(String(init.body));
    expect(body.requestId).toMatch(/^[0-9a-f-]{36}$/);
    expect(body.subdomain).toBe("test");
  });

  it("API の日本語エラーをそのまま利用する", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue(new Response(JSON.stringify({ error: { message: "指定された値が不正です" } }), { status: 400 })));
    await expect(api.list()).rejects.toEqual(new ApiError("指定された値が不正です", 400));
  });

  it("OperationのカスタムSSEイベントを状態として受け取る", () => {
    class EventSourceMock {
      static instance: EventSourceMock;
      listeners = new Map<string, (event: Event) => void>();
      onerror: (() => void) | null = null;
      close = vi.fn();
      constructor() { EventSourceMock.instance = this; }
      addEventListener(type: string, listener: (event: Event) => void) { this.listeners.set(type, listener); }
      emit(type: string, data: unknown) { this.listeners.get(type)?.(new MessageEvent("message", { data: JSON.stringify(data) })); }
    }
    vi.stubGlobal("EventSource", EventSourceMock);
    const onState = vi.fn();
    api.watchOperation("operations/test", onState, vi.fn());
    EventSourceMock.instance.emit("running", { type: "running", data: { message: "Composeでアプリを起動しています" } });
    EventSourceMock.instance.emit("succeeded", { type: "succeeded", data: { message: "" } });
    expect(onState).toHaveBeenNthCalledWith(1, { name: "operations/test", state: "running", errorMessage: "Composeでアプリを起動しています" });
    expect(onState).toHaveBeenNthCalledWith(2, { name: "operations/test", state: "succeeded", errorMessage: "" });
    expect(EventSourceMock.instance.close).toHaveBeenCalledTimes(1);
  });
});
