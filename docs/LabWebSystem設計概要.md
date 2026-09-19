# LabWebSystem（LWS）設計概要

本書はLWSの構成と責務を短く示す案内です。細かな契約は、各正本文書を参照してください。

## システムの目的

LWSは、信頼できる研究室・家庭内LANで、GitHub上のDocker Composeアプリを登録・検証・公開・管理するシステムです。アプリは`<subdomain>.<base-domain>`で公開されます。

## 正本と責務

| 対象 | 正本・責務 | 詳細 |
|---|---|---|
| アプリ状態 | BackendのSQLite | [Backend仕様書](Backend仕様書.md) |
| API契約 | `backend/openapi.yaml` | [Backend仕様書](Backend仕様書.md) |
| アプリ契約 | `lws.manifest.yaml`と`compose.yaml` | [Infrastructure仕様書](Infrastructure仕様書.md) |
| DNS・Reverse Proxy・Compose override | SQLiteから生成する派生物 | [Infrastructure仕様書](Infrastructure仕様書.md) |
| LWS本体のライフサイクル | `lwsctl`の6コマンド | [利用マニュアル](LWS%20v0.1.11利用マニュアル.md) |
| 実行環境 | Docker Compose | [Infrastructure仕様書](Infrastructure仕様書.md) |
| 開発・検証タスク | `mise` | [品質ゲート仕様](品質ゲート統合実装計画.md) |
| 受け入れテスト | `qa/suites/`のRobot Framework | [機能テストルール](機能テストルール.md) |

LWSは、パッケージ配布をGitHub Releases、Docker image配布をGHCR、SDK配布をGitHub Packagesへ委譲します。Linuxへのファイル配置と削除はAPTまたはDNFの責務です。

## コンポーネント

```text
利用者
  ├─ lwsctl ── LWS本体のCompose lifecycle
  └─ Dashboard ── Backend API経由のアプリ管理

Backend
  ├─ SQLite ── アプリ定義、設定、Operation、ログ
  ├─ Git / Compose / Docker ── アプリの取得・検証・実行
  └─ 生成設定 ── CoreDNS hosts、Caddyfile、アプリoverride

Infrastructure
  ├─ Caddy ── HTTP公開入口
  ├─ CoreDNS ── LWSドメインの名前解決
  └─ Docker Compose ── LWSと登録アプリの実行基盤
```

BackendだけがDocker socketを使います。登録アプリにはsocketを渡さず、アプリごとのedge networkとLWS所有labelで操作範囲を分離します。

## 開発からリリースまで

```text
mise install
  ↓
mise run verify
  ↓
mise run verify qa
  ↓
mise run verify release
  ↓
GitHub Actions
  ├─ lws-v<x.y.z> → LWS packageとGHCR image
  └─ sdk-v<x.y.z> → TypeScript SDK
```

通常CIは`mise run verify`を呼び出します。リリースWorkflowは品質ゲートを通過した後に、LWS packageとBackend・Dashboard imageのversionを揃えて公開します。

## LWSの利用フロー

1. APTまたはDNFでパッケージをインストールする。インストール時は起動しない。
2. `lwsctl start --domain <base-domain>`で設定を作成し、LWS本体を起動する。
3. DashboardからGitHub HTTPSリポジトリ、ref、subdomainを指定してアプリを登録する。
4. BackendがmanifestとComposeを検証し、設定完了まで`CONFIGURING`で保持する。
5. 環境変数とdevice bindingを設定してアプリを起動する。
6. `dashboard.<base-domain>`と各アプリURLから利用する。

登録直後は自動起動しません。同一app-idの変更は永続FIFOで待機し、Operation APIまたはSSEで進捗を確認します。

## 撤去と更新

- `lwsctl stop`は停止、`lwsctl down`は実行環境の削除を行い、設定と永続データを保持する。
- `lwsctl down --purge`は確認後、LWS所有かつinstallation IDが一致するresourceと設定・状態を削除する。
- `lwsctl update`はAPTまたはDNFへパッケージ更新を委譲し、対応するGHCR imageを更新する。更新前に起動中だった場合だけ再構成・再起動する。
- パッケージの直接削除時は、削除前hookがLWS実行環境を安全に停止する。

## 現在の範囲

Core v0.1.11は開発版です。Dashboardの基本的なアプリ管理画面は実装済みですが、TypeScript SDKと一部の実環境受け入れシナリオは開発中です。APIの認証・認可はなく、LWSは信頼できるLAN内で利用します。
