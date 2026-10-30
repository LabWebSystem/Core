import { expect, test } from "@playwright/test";

const emptyResourcePools = {
  devices: [],
  physicalDevices: [],
  volumes: [],
  networks: [],
};

test.beforeEach(async ({ page }) => {
  await page.route("**/api/v1/applications", async (route) => {
    if (route.request().method() === "GET") {
      await route.fulfill({ json: { applications: [] } });
      return;
    }
    await route.continue();
  });
  await page.route("**/api/v1/resource-pools**", (route) =>
    route.fulfill({ json: emptyResourcePools }),
  );
  await page.route("https://api.github.com/**", (route) =>
    route.fulfill({ json: [] }),
  );
});

test("空の台帳を表示する", async ({ page }) => {
  await page.goto("/");

  await expect(page.getByRole("heading", { name: "台帳にアプリを登録する" })).toBeVisible();
  await expect(page.getByText("Backend 接続中")).toBeVisible();
  await expect(page.getByRole("button", { name: "リソースプール" })).toBeVisible();
});

test("アプリ登録フォームから登録APIを呼び出す", async ({ page }) => {
  await page.route("**/api/v1/imports/github", (route) =>
    route.fulfill({ json: {
      importId: "550e8400-e29b-41d4-a716-446655440225",
      manifest: null,
      composeFiles: [{ name: "compose.yaml", services: ["web"], interfaces: [] }],
    } }),
  );
  await page.route("**/api/v1/applications", async (route) => {
    if (route.request().method() === "POST") {
      await route.fulfill({ status: 202, json: { name: "operations/550e8400-e29b-41d4-a716-446655440226" } });
    } else {
      await route.fulfill({ json: { applications: [] } });
    }
  });
  const request = page.waitForRequest(
    (candidate) =>
      candidate.url().endsWith("/api/v1/applications") && candidate.method() === "POST",
  );
  await page.goto("/");
  await page.getByPlaceholder("https://github.com/owner/repository").fill(
    "https://github.com/example/test",
  );
  await page.getByLabel("公開名").fill("test-app");
  await page.getByRole("button", { name: "取り込み内容を確認" }).click();
  await page.getByLabel("使用するComposeファイル").selectOption("compose.yaml");
  await page.getByLabel("アプリ名").fill("テスト");
  await page.getByLabel("公開サービス").selectOption("web");
  await page.getByLabel("公開ポート").fill("8080");
  await page.getByRole("button", { name: "この内容で登録" }).click();

  const body = JSON.parse((await request).postData() ?? "{}");
  expect(body).toMatchObject({
    importId: "550e8400-e29b-41d4-a716-446655440225",
    subdomain: "test-app",
    composeFile: "compose.yaml",
    displayName: "テスト",
    publicService: "web",
    publicPort: 8080,
  });
});
