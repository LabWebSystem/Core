import { ChakraProvider, defaultSystem } from "@chakra-ui/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMock = vi.hoisted(() => ({
  list: vi.fn(),
  get: vi.fn(),
  configuration: vi.fn(),
  resourcePools: vi.fn(),
  createPoolDevice: vi.fn(),
  create: vi.fn(),
  prepareGitImport: vi.fn(),
  prepareZipImport: vi.fn(),
  discardImport: vi.fn().mockResolvedValue(undefined),
  updateImport: vi.fn(),
  update: vi.fn(),
  action: vi.fn(),
  unregister: vi.fn(),
  purge: vi.fn(),
  saveConfiguration: vi.fn(),
  operation: vi.fn(),
  watchOperation: vi.fn(),
  tailLogs: vi.fn(),
}));
vi.mock("./api/client", () => ({
  api: apiMock,
  ApiError: class ApiError extends Error {},
}));
import { App, groupLogEntries } from "./App";

function renderApp() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return render(
    <ChakraProvider value={defaultSystem}>
      <QueryClientProvider client={client}>
        <App />
      </QueryClientProvider>
    </ChakraProvider>,
  );
}

describe("Dashboard デバッグルーム", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    Object.defineProperty(HTMLDialogElement.prototype, "showModal", {
      configurable: true,
      value() {
        this.setAttribute("open", "");
      },
    });
    Object.defineProperty(HTMLDialogElement.prototype, "close", {
      configurable: true,
      value() {
        this.removeAttribute("open");
      },
    });
    apiMock.list.mockResolvedValue([]);
    apiMock.configuration.mockResolvedValue({ variables: [] });
    apiMock.resourcePools.mockResolvedValue({
      devices: [],
      physicalDevices: [],
      volumes: [],
      networks: [],
    });
    apiMock.tailLogs.mockReturnValue(() => undefined);
    apiMock.watchOperation.mockReturnValue(() => undefined);
  });

  it("manifestのないGitHubアプリの公開先を指定して登録する", async () => {
    apiMock.create.mockResolvedValue({ name: "operations/create" });
    apiMock.prepareGitImport.mockResolvedValue({
      importId: "import-one",
      manifest: null,
      composeFiles: [{ name: "compose.yaml", services: ["web"], interfaces: [] }],
    });
    renderApp();
    await screen.findByText("台帳にアプリを登録する");
    fireEvent.change(screen.getByPlaceholderText("https://github.com/owner/repository"), {
      target: { value: "https://github.com/example/test" },
    });
    fireEvent.change(screen.getByLabelText("公開名"), { target: { value: "test-app" } });
    fireEvent.click(screen.getByRole("button", { name: "取り込み内容を確認" }));
    await screen.findByText(/manifestがありません/);
    fireEvent.change(screen.getByLabelText("使用するComposeファイル"), {
      target: { value: "compose.yaml" },
    });
    fireEvent.change(screen.getByLabelText("アプリ名"), { target: { value: "デモ" } });
    fireEvent.change(screen.getByLabelText("公開サービス"), { target: { value: "web" } });
    fireEvent.change(screen.getByLabelText("公開ポート"), { target: { value: "8080" } });
    fireEvent.click(screen.getByRole("button", { name: "この内容で登録" }));
    await waitFor(() =>
      expect(apiMock.create.mock.calls[0]?.[0]).toEqual({
        importId: "import-one",
        subdomain: "test-app",
        composeFile: "compose.yaml",
        overrideFiles: [],
        displayName: "デモ",
        description: "",
        publicService: "web",
        publicPort: 8080,
      }),
    );
  });

  it("ZIPのmanifestを登録値の初期値として表示する", async () => {
    apiMock.prepareZipImport.mockResolvedValue({
      importId: "import-zip",
      manifest: {
        apiVersion: "lws/v1",
        composeFile: "compose.prod.yaml",
        metadata: { name: "予約", description: "説明" },
        public: { service: "web", port: 3000 },
      },
      composeFiles: [{ name: "compose.prod.yaml", services: ["web"], interfaces: [] }],
    });
    renderApp();
    await screen.findByText("台帳にアプリを登録する");
    fireEvent.click(screen.getByLabelText("ZIPファイル"));
    fireEvent.change(screen.getByLabelText("公開名"), { target: { value: "zip-test" } });
    const file = new File(["zip"], "app.zip", { type: "application/zip" });
    fireEvent.change(screen.getByLabelText("取り込むZIPファイル"), { target: { files: [file] } });
    fireEvent.click(screen.getByRole("button", { name: "取り込み内容を確認" }));
    expect(await screen.findByText(/manifestを読み込みました/)).toBeTruthy();
    expect((screen.getByLabelText("使用するComposeファイル") as HTMLSelectElement).value).toBe(
      "compose.prod.yaml",
    );
    expect((screen.getByLabelText("アプリ名") as HTMLInputElement).value).toBe("予約");
    expect((screen.getByLabelText("公開ポート") as HTMLInputElement).value).toBe("3000");
  });

  it("ZIPアプリの更新内容を確認してから更新する", async () => {
    const app = {
      name: "applications/zip",
      subdomain: "zip",
      repositoryUrl: "",
      ref: "",
      sourceKind: "zip",
      desiredState: "STOPPED",
      observedState: "STOPPED",
      registrationState: "ACTIVE",
      observedAt: "2026-08-23T00:00:00Z",
      reconciling: false,
      etag: "tag",
    };
    apiMock.list.mockResolvedValue([app]);
    apiMock.prepareZipImport.mockResolvedValue({
      importId: "zip-update",
      manifest: {
        apiVersion: "lws/v1",
        composeFile: "compose.yaml",
        metadata: { name: "更新", description: "" },
        public: { service: "web", port: 8080 },
      },
      composeFiles: [{ name: "compose.yaml", services: ["web"], interfaces: [] }],
    });
    apiMock.updateImport.mockResolvedValue({ name: "operations/update" });
    apiMock.operation.mockResolvedValue({
      name: "operations/update",
      state: "succeeded",
      kind: "sync",
    });
    renderApp();
    fireEvent.click(await screen.findByRole("button", { name: "ZIPで更新" }));
    fireEvent.change(screen.getByLabelText("取り込むZIPファイル"), {
      target: { files: [new File(["zip"], "app.zip")] },
    });
    fireEvent.click(screen.getByRole("button", { name: "取り込み内容を確認" }));
    await screen.findByText(/manifestを読み込みました/);
    expect(apiMock.updateImport).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "この内容で更新" }));
    await waitFor(() =>
      expect(apiMock.updateImport).toHaveBeenCalledWith(
        app,
        expect.objectContaining({
          importId: "zip-update",
          composeFile: "compose.yaml",
          displayName: "更新",
          publicService: "web",
          publicPort: 8080,
        }),
      ),
    );
  });

  it("取り込み失敗を表示し登録しない", async () => {
    apiMock.prepareZipImport.mockRejectedValue(new Error("manifestのschemaが不正です"));
    renderApp();
    await screen.findByText("台帳にアプリを登録する");
    fireEvent.click(screen.getByLabelText("ZIPファイル"));
    fireEvent.change(screen.getByLabelText("公開名"), { target: { value: "zip-test" } });
    fireEvent.change(screen.getByLabelText("取り込むZIPファイル"), {
      target: { files: [new File(["zip"], "app.zip")] },
    });
    fireEvent.click(screen.getByRole("button", { name: "取り込み内容を確認" }));
    expect((await screen.findByRole("alert")).textContent).toContain("manifestのschemaが不正です");
    expect(apiMock.create).not.toHaveBeenCalled();
  });

  it("取り込み元へ戻ると一時データを破棄する", async () => {
    apiMock.prepareGitImport.mockResolvedValue({
      importId: "temporary-import",
      manifest: null,
      composeFiles: [{ name: "compose.yaml", services: ["web"], interfaces: [] }],
    });
    renderApp();
    await screen.findByText("台帳にアプリを登録する");
    fireEvent.change(screen.getByLabelText("GitHub リポジトリ"), {
      target: { value: "https://github.com/example/test" },
    });
    fireEvent.change(screen.getByLabelText("公開名"), { target: { value: "test" } });
    fireEvent.click(screen.getByRole("button", { name: "取り込み内容を確認" }));
    await screen.findByText(/manifestがありません/);
    fireEvent.click(screen.getByRole("button", { name: "取り込み元に戻る" }));
    expect(apiMock.discardImport).toHaveBeenCalledWith("temporary-import");
    expect(screen.getByLabelText("GitHub リポジトリ")).toBeTruthy();
  });

  it("登録済みアプリがあっても登録画面を開ける", async () => {
    apiMock.list.mockResolvedValue([
      {
        name: "applications/test",
        subdomain: "test",
        repositoryUrl: "https://github.com/example/test",
        ref: "main",
        desiredState: "RUNNING",
        observedState: "RUNNING",
        registrationState: "ACTIVE",
        observedAt: "2026-08-23T00:00:00Z",
        reconciling: false,
        etag: "tag",
      },
    ]);
    renderApp();
    await screen.findByText("完全削除");
    fireEvent.click(screen.getByRole("button", { name: "登録する" }));
    expect(await screen.findByText("台帳にアプリを登録する")).toBeTruthy();
  });

  it("登録解除済みアプリを表示し、再登録と完全削除を選べる", async () => {
    apiMock.list.mockResolvedValue([
      {
        name: "applications/test",
        subdomain: "test",
        repositoryUrl: "https://github.com/example/test",
        ref: "main",
        desiredState: "STOPPED",
        observedState: "STOPPED",
        registrationState: "UNREGISTERED",
        observedAt: "2026-08-23T00:00:00Z",
        reconciling: false,
        etag: "tag",
      },
    ]);
    renderApp();
    expect((await screen.findAllByText("登録解除済み")).length).toBeGreaterThan(0);
    expect(screen.getByRole("button", { name: "再登録" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "完全削除" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "登録解除" })).toBeNull();
    expect(screen.queryByRole("button", { name: "同期" })).toBeNull();
  });

  it("連続したJSONログを一つの構造化ログにまとめる", () => {
    const entries = ["{", `  "service": "web",`, `  "status": "ready"`, "}"].map(
      (message, index) => ({
        id: String(index),
        cursor: String(index),
        occurredAt: "2026-08-23T00:00:00Z",
        level: "info" as const,
        component: "application" as const,
        service: "web",
        containerName: "web-1",
        message: `[Compose検証] ${message}`,
      }),
    );
    const grouped = groupLogEntries(entries);
    expect(grouped).toHaveLength(1);
    expect(grouped[0].json).toBe('{"service":"web","status":"ready"}');
    expect(grouped[0].lineCount).toBe(4);
  });

  it("完全削除は確認するまで API を呼ばない", async () => {
    apiMock.list.mockResolvedValue([
      {
        name: "applications/test",
        subdomain: "test",
        repositoryUrl: "https://github.com/example/test",
        ref: "main",
        desiredState: "RUNNING",
        observedState: "RUNNING",
        registrationState: "ACTIVE",
        observedAt: "2026-08-23T00:00:00Z",
        reconciling: false,
        etag: "tag",
      },
    ]);
    apiMock.purge.mockResolvedValue({ name: "operations/purge" });
    renderApp();
    await screen.findByText("完全削除");
    fireEvent.click(screen.getByRole("button", { name: "完全削除" }));
    expect(apiMock.purge).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "実行する" }));
    await waitFor(() => expect(apiMock.purge).toHaveBeenCalledTimes(1));
  });

  it("完全削除のバックグラウンド段階を表示する", async () => {
    apiMock.list.mockResolvedValue([
      {
        name: "applications/test",
        subdomain: "test",
        repositoryUrl: "https://github.com/example/test",
        ref: "main",
        desiredState: "STOPPED",
        observedState: "STOPPED",
        registrationState: "UNREGISTERED",
        observedAt: "2026-08-23T00:00:00Z",
        reconciling: false,
        latestOperation: "operation-purge",
        etag: "tag",
      },
    ]);
    apiMock.operation.mockResolvedValue({
      name: "operations/operation-purge",
      kind: "purge",
      state: "running",
      phase: "runtime_prepare",
      displayMessage: "アプリのDocker volumeを削除しています",
      errorMessage: "",
      createdAt: "2026-08-23T00:00:00Z",
      updatedAt: "2026-08-23T00:01:00Z",
    });
    renderApp();
    expect(await screen.findByText("完全削除: 実行中")).toBeTruthy();
    expect(screen.getByText("段階: 実行設定")).toBeTruthy();
    expect(screen.getByText("アプリのDocker volumeを削除しています")).toBeTruthy();
    expect(screen.getByRole("button", { name: "完全削除" }).hasAttribute("disabled")).toBe(true);
  });

  it("未完了の操作とアプリの実行状態を表示し、操作を無効化する", async () => {
    apiMock.list.mockResolvedValue([
      {
        name: "applications/test",
        subdomain: "test",
        repositoryUrl: "https://github.com/example/test",
        ref: "main",
        desiredState: "RUNNING",
        observedState: "ERROR",
        registrationState: "ACTIVE",
        observedAt: "2026-08-23T00:00:00Z",
        reconciling: true,
        latestOperation: "operation-1",
        etag: "tag",
      },
    ]);
    apiMock.operation.mockResolvedValue({
      name: "operations/operation-1",
      kind: "rebuild",
      state: "running",
      createdAt: "2026-08-23T00:00:00Z",
      updatedAt: "2026-08-23T00:01:00Z",
    });
    renderApp();
    await screen.findByText("リビルド: 実行中");
    expect(screen.getByText("実行状態").nextElementSibling?.textContent).toContain("異常");
    expect(screen.getByRole("button", { name: "開始" }).hasAttribute("disabled")).toBe(true);
  });

  it("ページを読み込み直しても直近の完了Operationを表示する", async () => {
    apiMock.list.mockResolvedValue([
      {
        name: "applications/test",
        subdomain: "test",
        repositoryUrl: "https://github.com/example/test",
        ref: "main",
        desiredState: "RUNNING",
        observedState: "RUNNING",
        registrationState: "ACTIVE",
        observedAt: "2026-08-23T00:00:00Z",
        reconciling: false,
        latestOperation: "operation-2",
        etag: "tag",
      },
    ]);
    apiMock.operation.mockResolvedValue({
      name: "operations/operation-2",
      kind: "start",
      state: "succeeded",
      createdAt: "2026-08-23T00:00:00Z",
      updatedAt: "2026-08-23T00:01:00Z",
    });
    renderApp();
    await screen.findByText("開始: 完了");
    expect(screen.getByRole("button", { name: "開始" }).hasAttribute("disabled")).toBe(false);
  });

  it("Compose由来のリソースと、値を伏せたsecretの保持を表示する", async () => {
    apiMock.list.mockResolvedValue([
      {
        name: "applications/test",
        subdomain: "test",
        repositoryUrl: "https://github.com/example/test",
        ref: "main",
        desiredState: "RUNNING",
        observedState: "RUNNING",
        registrationState: "ACTIVE",
        observedAt: "2026-08-23T00:00:00Z",
        reconciling: false,
        etag: "tag",
      },
    ]);
    apiMock.configuration.mockResolvedValue({
      variables: [
        {
          name: "PORT",
          isSecret: false,
          configured: true,
          required: false,
          hasDefault: true,
          value: "3000",
        },
        {
          name: "TOKEN",
          isSecret: true,
          configured: true,
          required: true,
          hasDefault: false,
        },
      ],
      volumes: [{ name: "app-data" }],
      network: {
        name: "lws-app-test-edge",
        purpose: "公開サービスとLWSのReverse Proxyだけを接続",
      },
      devices: [],
      ready: true,
    });
    apiMock.saveConfiguration.mockResolvedValue({
      name: "operations/configure",
    });
    apiMock.operation.mockResolvedValue({
      name: "operations/configure",
      kind: "configure",
      state: "queued",
      createdAt: "2026-08-23T00:00:00Z",
      updatedAt: "2026-08-23T00:00:00Z",
    });
    renderApp();
    expect(await screen.findByText("app-data")).toBeTruthy();
    expect(screen.getByText("lws-app-test-edge")).toBeTruthy();
    expect(screen.getByText("デバイスは不要")).toBeTruthy();
    expect(screen.getByPlaceholderText("設定済み（変更する場合のみ入力）")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "保存" }));
    fireEvent.click(screen.getByRole("button", { name: "実行する" }));
    await waitFor(() =>
      expect(apiMock.saveConfiguration).toHaveBeenCalledWith(
        expect.anything(),
        {
          PORT: { value: "3000", secret: false },
          TOKEN: { secret: true, keep: true },
        },
        [],
      ),
    );
  });

  it("ComposeのdeviceをLWSデバイスプールから割り当てて保存する", async () => {
    apiMock.list.mockResolvedValue([
      {
        name: "applications/test",
        subdomain: "test",
        repositoryUrl: "https://github.com/example/test",
        ref: "main",
        desiredState: "STOPPED",
        observedState: "STOPPED",
        registrationState: "CONFIGURING",
        observedAt: "2026-08-23T00:00:00Z",
        reconciling: false,
        etag: "tag",
      },
    ]);
    apiMock.configuration.mockResolvedValue({
      variables: [],
      volumes: [],
      network: {
        name: "lws-app-test-edge",
        purpose: "公開サービスとLWSのReverse Proxyだけを接続",
      },
      devices: [
        {
          service: "reader",
          sourceHint: "/dev/hidraw2",
          targetPath: "/dev/lws/card-reader",
          permissions: "rw",
          configured: false,
        },
      ],
      ready: false,
    });
    apiMock.resourcePools.mockResolvedValue({
      devices: [
        {
          id: "device-reader",
          name: "NFC reader",
          stableId: "serial",
          currentPath: "/dev/hidraw7",
          status: "connected",
        },
      ],
      physicalDevices: [],
      volumes: [],
      networks: [],
    });
    apiMock.saveConfiguration.mockResolvedValue({
      name: "operations/configure",
    });
    apiMock.operation.mockResolvedValue({
      name: "operations/configure",
      kind: "configure",
      state: "queued",
      createdAt: "",
      updatedAt: "",
    });
    renderApp();
    const select = await screen.findByRole("combobox");
    fireEvent.change(select, { target: { value: "device-reader" } });
    fireEvent.click(screen.getByRole("button", { name: "登録" }));
    fireEvent.click(screen.getByRole("button", { name: "実行する" }));
    await waitFor(() =>
      expect(apiMock.saveConfiguration).toHaveBeenCalledWith(expect.anything(), {}, [
        {
          service: "reader",
          targetPath: "/dev/lws/card-reader",
          deviceId: "device-reader",
        },
      ]),
    );
  });
});
