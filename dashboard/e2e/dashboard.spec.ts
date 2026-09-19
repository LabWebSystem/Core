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
  const request = page.waitForRequest(
    (candidate) =>
      candidate.url().endsWith("/api/v1/applications") && candidate.method() === "POST",
  );
  await page.goto("/");
  await page.getByPlaceholder("https://github.com/owner/repository").fill(
    "https://github.com/example/test",
  );
  await page.getByRole("button", { name: "検証して登録" }).click();

  const body = JSON.parse((await request).postData() ?? "{}");
  expect(body).toMatchObject({
    repositoryUrl: "https://github.com/example/test",
    ref: "main",
    subdomain: "test",
  });
});
