# LabWebSystem

LabWebSystem（LWS）は、研究室や家庭内LANでWebアプリを公開・管理するための仕組みです。

アプリはGitHubリポジトリから登録します。LWSがDocker Composeでアプリを動かし、アプリごとのURLをLAN内へ公開します。

> 現在のLWSは v0.1.11 です。CLI、Backend、Dashboardによるアプリ管理を使えます。TypeScript SDKは、まだ開発中です。

## できること

- GitHubリポジトリにあるWebアプリを登録する
- Docker Composeでアプリを起動・停止・更新する
- `app-name.example.internal`の形でアプリごとのURLを公開する
- アプリの設定値、secret、ログ、実行状態を管理する
- アプリごとの名前付きVolume、edge Network、デバイス割り当てを設定する
- `dashboard.<base-domain>`のDashboardから、アプリを日本語で登録・操作する
- LWS本体を起動、停止、再構成、更新する

## はじめる

### 必要なもの

- Ubuntu系またはAlmaLinux系のLinux
- Docker EngineとDocker Compose plugin
- 空いている80/tcp、53/tcp、53/udpポート
- GitHub Releasesへ接続できるネットワーク

LAN内の端末からアプリ名でアクセスするには、端末がLWSのDNSを使うようにDHCPまたはDNSを設定します。

### インストール

インストーラーはGitHub ReleasesからOSに合うパッケージを取得してインストールします。インストールしただけではLWSは起動しません。

```sh
curl -fsSL https://raw.githubusercontent.com/LabWebSystem/Core/main/scripts/install.sh | sudo bash
```

フォークからインストールする場合は、`LWS_REPOSITORY=owner/repository`を設定します。

### 初回起動

ベースドメインを決めて起動します。

```sh
sudo lwsctl start --domain example.internal
sudo lwsctl status
```

たとえば、`reserve`というアプリは次のURLで公開されます。

```text
reserve.example.internal
```

## LWSを操作する

| コマンド | 内容 |
|---|---|
| `lwsctl start --domain <domain>` | 初回設定またはLWSの起動 |
| `lwsctl stop` | LWSを停止。設定と保存データは残る |
| `lwsctl status` | 設定とコンテナの状態を表示 |
| `lwsctl rebuild` | 設定を作り直して実行環境を再構成 |
| `lwsctl update` | パッケージとDockerイメージを更新。同じdigestのイメージは再取得しない |
| `lwsctl down` | 実行環境を削除。設定と保存データは残る |

設定や保存データも含めて削除する場合は、次を実行します。

```sh
sudo lwsctl down --purge
```

詳しい操作、アプリの登録方法、完全削除については、[利用マニュアル](docs/LWS%20v0.1.11利用マニュアル.md)を参照してください。

## アプリを公開する

登録するGitHubリポジトリには、次のファイルが必要です。

```text
compose.yaml
lws.manifest.yaml
```

`lws.manifest.yaml`には、公開するCompose serviceとportを書きます。

```yaml
apiVersion: lws/v1
metadata:
  name: 予約システム
  description: 研究室の設備予約を管理するアプリ
public:
  service: web
  port: 3000
```

LWSはリポジトリを取得して内容を確認します。登録直後は`CONFIGURING`（設定待ち）となり、Dashboardで環境変数とデバイス割り当てを確認してから開始します。アプリの登録・設定・操作はBackend APIから行います。APIの詳しい使い方は[利用マニュアル](docs/LWS%20v0.1.11利用マニュアル.md)を参照してください。

### Backend APIの概要

APIのベースパスは`/api/v1`です。変更操作はすぐに`Operation`を返し、進捗は`GET /operations/{operation}`またはSSEの`GET /operations/{operation}:watch`で確認します。

主なリソースと操作は次のとおりです。

| メソッド | パス | 内容 |
| --- | --- | --- |
| `GET` / `POST` | `/applications` | アプリ一覧、アプリ登録 |
| `GET` / `PATCH` / `DELETE` | `/applications/{application}` | 取得、更新、登録解除 |
| `POST` | `/applications/{application}:start`、`:stop`、`:sync`、`:rebuild`、`:register`、`:purge` | アプリ操作 |
| `GET` / `PATCH` | `/applications/{application}/configuration` | 環境変数・デバイス設定 |
| `GET` | `/resource-pools` | Volume、Network、デバイス候補・登録済みデバイス |
| `POST` | `/resource-pools/devices` | 物理デバイスをLWSプールへ登録 |
| `GET` | `/applications/{application}/logEntries` | 永続ログ取得 |

登録直後のアプリは自動起動せず、設定完了後に`:start`を実行します。完全削除は`confirm:true`を付けた`:purge`で行います。

## 開発

このリポジトリでは、開発に必要なツールとコマンドを`mise`で管理します。

```sh
mise install
mise run verify          # 普段の開発・通常CI用。約数秒
mise run verify qa       # DockerとRobot QAを含む確認
mise run verify release  # リリース前の確認とパッケージ生成
mise run dev             # 開発用Composeを起動
mise run dev-dashboard   # HMR対応のDashboardと実Backendを起動
mise run dev-dashboard-build # 初回またはBackend変更時に再ビルドして起動
```

### Dashboardを実際に操作する

`mise run dev-dashboard`は、`dashboard/src`をマウントしたVite開発サーバーを`dashboard.lws.localhost:18180`で公開します。保存したUI変更はHMRで即時反映され、Caddyの同一Originプロキシを通じて実Backendへ接続します。通常のLWSとは別のCompose project、volume、networkと、80/53番以外のportを使用します。

初回起動時、または`backend/`・Dockerfile・Compose構成を変更したときだけ、先に`mise run dev-dashboard-build`でBackendを再ビルドします。BackendのGoモジュール・コンパイルキャッシュはBuildKitで再利用します。`dashboard/src`だけの変更には再ビルド不要です。

起動後にブラウザで次を開きます。

```text
http://dashboard.lws.localhost:18180
```

`.localhost`はローカル端末へ解決されるため、hostsの編集は不要です。停止してデータを残すには`Ctrl+C`、コンテナとnetworkを削除するには次を実行します。

```sh
mise run dev-dashboard-down
```

登録したテストアプリとDashboardの開発データも最初からやり直す場合だけ、専用volumeを含めて削除します。

```sh
mise run dev-dashboard-reset
```

品質ゲートの結果とログは、`test/result/YYYY-MM-DD-verify-result.md`に保存されます。
QAとDashboardのイメージ検証は、実行中に新しく取得・作成したDockerイメージを終了時に削除します。

| 作業 | コマンド |
|---|---|
| Coreのテスト | `mise run test <target>` |
| 受け入れテスト | `mise run qa`、`mise run qa-current`、`mise run qa-lifecycle`、`mise run qa-live` |
| パッケージ生成 | `mise run package` |
| バージョン確認・変更 | `mise run version [core\|sdk] [version]` |
| 公開 | `mise run release <core\|sdk\|all>` |

品質ゲートとテストの役割は、[機能テストルール](docs/機能テストルール.md)を参照してください。

## リポジトリ構成

| 場所 | 内容 |
|---|---|
| `cmd/lwsctl/` | LWSを操作するCLI |
| `backend/` | アプリ管理とDocker操作を行うBackend API |
| `dashboard/` | アプリ管理DashboardのReact実装 |
| `sdk/` | 外部アプリ向けTypeScript SDK |
| `infrastructure/` | LWS本体のDocker Compose、DNS、Reverse Proxy設定 |
| `packaging/` | `.deb`・`.rpm`パッケージ定義 |
| `qa/` | Robot Frameworkの受け入れテスト |

## 注意点

- 管理APIには、まだ認証・認可がありません。信頼できるLAN内だけで使ってください。
- TypeScript SDKは、実用機能の実装途中です。
- LAN端末へLWSのDNSを配るDHCP・DNS設定は、利用するネットワーク側で行います。

## 関連資料

- [利用マニュアル](docs/LWS%20v0.1.11利用マニュアル.md)
- [設計概要](docs/LabWebSystem設計概要.md)
- [Infrastructure仕様書](docs/Infrastructure仕様書.md)
- [Backend仕様書](docs/Backend仕様書.md)
- [機能テストルール](docs/機能テストルール.md)
- [LICENSE](LICENSE)
