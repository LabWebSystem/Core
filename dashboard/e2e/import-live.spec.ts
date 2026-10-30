import { execFileSync } from "node:child_process";
import { expect, test } from "@playwright/test";

const backend = process.env.LWS_E2E_BACKEND;
const installation = process.env.LWS_E2E_INSTALLATION;

test.skip(!backend || !installation, "隔離されたZIP取り込み環境で実行します");

function archive(version: string): Buffer {
  const script = [
    "import io,sys,zipfile",
    "buf=io.BytesIO()",
    "z=zipfile.ZipFile(buf,'w',zipfile.ZIP_DEFLATED)",
    `z.writestr('compose.yaml', 'services:\\n  web:\\n    image: caddy:2.10-alpine\\n    expose: [80]\\n    environment:\\n      APP_VERSION: \\\"${version}\\\"\\n')`,
    "z.close()",
    "sys.stdout.buffer.write(buf.getvalue())",
  ].join(";");
  return execFileSync("python3", ["-c", script]);
}

test("ブラウザからZIP登録・起動・更新する", async ({ page, request }) => {
  test.setTimeout(120_000);
  await page.route("**/api/v1/**", async (route) => {
    const url = new URL(route.request().url());
    if (url.pathname.endsWith("/logEntries:watch")) {
      await route.fulfill({ status: 200, contentType: "text/event-stream", body: "" });
      return;
    }
    const response = await route.fetch({ url: `${backend}${url.pathname}${url.search}` });
    await route.fulfill({ response });
  });
  await page.goto("/");
  await page.getByLabel("ZIPファイル", { exact: true }).check();
  await page.getByLabel("取り込むZIPファイル").setInputFiles({
    name: "app.zip", mimeType: "application/zip", buffer: archive("1"),
  });
  await page.getByLabel("公開名").fill("browser-e2e");
  await page.getByRole("button", { name: "取り込み内容を確認" }).click();
  await expect(page.getByText(/manifestがありません/)).toBeVisible();
  await page.getByLabel("使用するComposeファイル").selectOption("compose.yaml");
  await page.getByLabel("アプリ名").fill("ブラウザテスト");
  await page.getByLabel("公開サービス").selectOption("web");
  await page.getByLabel("公開ポート").fill("80");
  await page.getByRole("button", { name: "この内容で登録" }).click();
  await expect(page.getByRole("button", { name: "開始" })).toBeEnabled({ timeout: 30_000 });
  const applications = await (await request.get(`${backend}/api/v1/applications`)).json();
  const id = String(applications.applications[0].name).replace("applications/", "");
  await page.getByRole("button", { name: "開始" }).click();
  await expect.poll(async () => {
    const result = await (await request.get(`${backend}/api/v1/applications`)).json();
    return result.applications[0]?.observedState;
  }, { timeout: 60_000 }).toBe("RUNNING");

  await page.getByRole("button", { name: "ZIPで更新" }).click();
  await page.getByLabel("取り込むZIPファイル").setInputFiles({
    name: "update.zip", mimeType: "application/zip", buffer: archive("2"),
  });
  await page.getByRole("button", { name: "取り込み内容を確認" }).click();
  await page.getByLabel("使用するComposeファイル").selectOption("compose.yaml");
  await page.getByLabel("アプリ名").fill("ブラウザテスト");
  await page.getByLabel("公開サービス").selectOption("web");
  await page.getByLabel("公開ポート").fill("80");
  await page.getByRole("button", { name: "この内容で更新" }).click();
  await expect.poll(() => {
    const ids = execFileSync("docker", ["ps", "-q", "--filter", `label=com.labwebsystem.installation-id=${installation}`, "--filter", `label=com.labwebsystem.app-id=${id}`], { encoding: "utf8" }).trim();
    if (!ids) return "";
    try {
      return execFileSync("docker", ["inspect", "--format", "{{json .Config.Env}}", ids.split("\n")[0]], { encoding: "utf8" });
    } catch {
      // 更新中はpsとinspectの間にComposeがcontainerを入れ替えることがあります。
      return "";
    }
  }, { timeout: 60_000 }).toContain("APP_VERSION=2");
});
