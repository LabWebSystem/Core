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
});
