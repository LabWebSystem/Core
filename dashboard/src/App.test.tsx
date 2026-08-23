import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMock = vi.hoisted(() => ({
  list: vi.fn(), get: vi.fn(), configuration: vi.fn(), create: vi.fn(), update: vi.fn(), action: vi.fn(), unregister: vi.fn(), purge: vi.fn(), saveConfiguration: vi.fn(), operation: vi.fn(), watchOperation: vi.fn(), tailLogs: vi.fn(),
}));
vi.mock("./api/client", () => ({ api: apiMock, ApiError: class ApiError extends Error {} }));
import { App } from "./App";

function renderApp() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<ChakraProvider value={defaultSystem}><QueryClientProvider client={client}><App /></QueryClientProvider></ChakraProvider>);
}

describe("Dashboard デバッグルーム", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", { configurable: true, value() { this.setAttribute("open", ""); } });
    Object.defineProperty(HTMLDialogElement.prototype, "close", { configurable: true, value() { this.removeAttribute("open"); } });
    apiMock.list.mockResolvedValue([]);
    apiMock.configuration.mockResolvedValue({ variables: [] });
    apiMock.tailLogs.mockReturnValue(() => undefined);
    apiMock.watchOperation.mockReturnValue(() => undefined);
  });

  it("空の台帳からテストアプリを登録する", async () => {
    apiMock.create.mockResolvedValue({ name: "operations/create" });
    renderApp();
    await screen.findByText("台帳にアプリを登録する");
    fireEvent.change(screen.getByPlaceholderText("https://github.com/owner/repository"), { target: { value: "https://github.com/example/test" } });
    fireEvent.submit(screen.getByRole("button", { name: "検証して登録" }).closest("form")!);
    await waitFor(() => expect(apiMock.create.mock.calls[0]?.[0]).toEqual({ repositoryUrl: "https://github.com/example/test", ref: "main", subdomain: "test-app" }));
  });

  it("完全削除は確認するまで API を呼ばない", async () => {
    apiMock.list.mockResolvedValue([{ name: "applications/test", subdomain: "test", repositoryUrl: "https://github.com/example/test", ref: "main", desiredState: "RUNNING", observedState: "RUNNING", registrationState: "ACTIVE", observedAt: "2026-08-23T00:00:00Z", reconciling: false, etag: "tag" }]);
    apiMock.purge.mockResolvedValue({ name: "operations/purge" });
    renderApp();
    await screen.findByText("完全削除");
    fireEvent.click(screen.getByRole("button", { name: "完全削除" }));
    expect(apiMock.purge).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "実行する" }));
    await waitFor(() => expect(apiMock.purge).toHaveBeenCalledTimes(1));
  });
});
