# LabWebSystem

LabWebSystem（LWS）は、研究室や家庭内LANでWebアプリを公開・管理するための仕組みです。

アプリはGitHubリポジトリまたはZIPファイルから登録します。LWSがDocker Composeでアプリを動かし、アプリごとのURLをLAN内へ公開します。

> Core v0.1.12を公開しました。GitHubまたはZIPからのアプリ登録、`.local` mDNSによる公開に対応しています。TypeScript SDKは開発中です。

## できること

- GitHubリポジトリまたはZIPファイルにあるWebアプリを登録する
- Docker Composeでアプリを起動・停止・更新する
- `app-name-example.local`の形でアプリごとのURLを公開する
- アプリの設定値、secret、ログ、実行状態を管理する
- アプリごとの名前付きVolume、edge Network、デバイス割り当てを設定する
- `dashboard-<base-domain>`のDashboardから、アプリを日本語で登録・操作する
- LWS本体を起動、停止、再構成、更新する

## はじめる

### 必要なもの

- Ubuntu系またはAlmaLinux系のLinux
- 現在のリリース成果物はamd64（RPMではx86_64）向け
- Docker EngineとDocker Compose plugin
- 空いている80/tcp、5353/udpポート
- GitHub Releasesへ接続できるネットワーク

LAN内の端末からアプリ名でアクセスするには、端末のmDNSを有効にし、LWSホストと同じLANへ接続します。ルーターのDNS設定は不要です。

### インストール

インストーラーはGitHub ReleasesからOSに合うパッケージを取得してインストールします。インストールしただけではLWSは起動しません。

```sh
curl -fsSL https://raw.githubusercontent.com/LabWebSystem/Core/main/scripts/install.sh | sudo bash
```

フォークからインストールする場合は、`LWS_REPOSITORY=owner/repository`を設定します。

### 初回起動

ベースドメインを決めて起動します。

```sh
sudo lwsctl start --domain example.local
sudo lwsctl status
```

ベースドメインは`.local`で終わる名前を指定します。mDNSを通さないネットワークを越えた名前解決には対応しません。

以前のベースドメインから移行する場合は、次のように新しい名前を指定します。

```sh
sudo lwsctl start --domain home.local --force
```

たとえば、`reserve`というアプリは次のURLで公開されます。

```text
reserve-example.local
```

たとえばベースドメインが`test.local`なら、Dashboardは`dashboard-test.local`、アプリ`reserve`は`reserve-test.local`で開きます。複数ラベルのベースドメイン`lab.home.local`は`lab-home.local`として公開名に含めます。

## LWSを操作する

| コマンド | 内容 |
|---|---|
| `lwsctl start --domain <domain>` | 初回設定またはLWSの起動 |
| `lwsctl stop [--recursive]` | LWSを停止。`--recursive`指定時は子のアプリcontainerも停止する |
| `lwsctl status` | lwsctlのバージョン、設定とコンテナの状態を表示 |
| `lwsctl version` | lwsctlのバージョンを表示 |
| `lwsctl rebuild` | 設定を作り直して実行環境を再構成 |
| `lwsctl update` | パッケージとDockerイメージを更新。同じdigestのイメージは再取得しない |
| `lwsctl down [--recursive]` | LWSの実行環境を削除。`--recursive`指定時は子のアプリcontainerも削除する |

設定や保存データも含めて削除する場合は、次を実行します。

```sh
sudo lwsctl down --purge
```

通常の`stop`と`down`はLWS本体だけを対象にします。子のアプリも対象にする場合は`-r`または`--recursive`を追加します。`down --recursive --purge`では、子のアプリのLWS所有volumeも削除します。

詳しい操作、アプリの登録方法、完全削除については、[利用マニュアル](docs/LWS%20v0.1.12利用マニュアル.md)を参照してください。

## アプリを公開する

登録するアプリにはComposeファイルが必要です。ZIPはファイルを直下、または直下の単一フォルダに配置してください。

```text
compose.yaml
lws.manifest.yaml
```

`lws.manifest.yaml`は任意です。記載するとアプリ名と公開先が登録画面に反映されます。

```yaml
apiVersion: lws/v1
composeFile: compose.yaml
metadata:
  name: 予約システム
  description: 研究室の設備予約を管理するアプリ
public:
  service: web
  port: 3000
```

DashboardでGitHubのURLとref、またはZIPを指定し、取り込み内容を確認します。manifestがない場合はアプリ名、Composeファイル、公開serviceとportを指定します。登録直後は`CONFIGURING`（設定待ち）となり、環境変数とデバイス割り当てを確認してから開始します。ZIPで登録したアプリは、新しいZIPを指定して更新できます。

### Backend APIの概要

APIのベースパスは`/api/v1`です。変更操作はすぐに`Operation`を返し、進捗は`GET /operations/{operation}`またはSSEの`GET /operations/{operation}:watch`で確認します。

OpenAPIの仕様は`http://api-<base-domain>/openapi.json`または`http://dashboard-<base-domain>/openapi.json`で取得でき、Swagger UIは`http://api-<base-domain>/swagger/`または`http://dashboard-<base-domain>/swagger/`で確認できます。

主なリソースと操作は次のとおりです。

| メソッド | パス | 内容 |
| --- | --- | --- |
| `GET` / `POST` | `/applications` | アプリ一覧、アプリ登録 |
| `POST` | `/imports/github`、`/imports/zip` | GitHubまたはZIPを取得して登録候補を確認 |
| `POST` | `/applications/{application}/imports` | 確認済みZIPによるアプリ更新 |
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
gh auth login            # 初回のみ
mise run verify          # GitHub Actionsでfast検証を実行
mise run verify full     # GitHub Actionsで全品質ゲートを実行
mise run verify release  # fullの互換指定
mise run package         # ローカルでパッケージを生成
mise run dev             # 開発用Composeを起動
mise run dev-dashboard   # HMR対応のDashboardと実Backendを起動
mise run dev-dashboard-build # 初回またはBackend変更時に再ビルドして起動
```

### Dashboardを実際に操作する

`mise run dev-dashboard`は、Backendがシステムアプリとして登録するVite開発サーバーへ`dashboard-lws.localhost:18180`で接続します。`dashboard/src`をマウントするため、保存したUI変更はHMRで即時反映されます。通常のLWSとは別のCompose project、volume、networkと、80/5353番以外のportを使用します。

初回起動時、または`backend/`・Dockerfile・Compose構成を変更したときだけ、先に`mise run dev-dashboard-build`でBackendを再ビルドします。BackendのGoモジュール・コンパイルキャッシュはBuildKitで再利用します。`dashboard/src`だけの変更には再ビルド不要です。

起動後にブラウザで次を開きます。

```text
http://dashboard-lws.localhost:18180
```

`.localhost`はローカル端末へ解決されるため、hostsの編集は不要です。停止してデータを残すには`Ctrl+C`、コンテナとnetworkを削除するには次を実行します。

```sh
mise run dev-dashboard-down
```

登録したテストアプリとDashboardの開発データも最初からやり直す場合だけ、専用volumeを含めて削除します。

```sh
mise run dev-dashboard-reset
```

`mise run verify`はGitHub CLIを使ってGitHub Actionsを起動し、終了まで待機します。作業ツリーがcleanで、現在のコミットが追跡先ブランチへpush済みであることが必要です。引数なしはfast、`full`・`qa`・`release`は全品質ゲートを実行し、Actions画面からの手動起動も初期値をfullとします。PR・ブランチpushは自動でfast検証します。結果artifactは`test/result/actions/<run-id>/`へ取得されます。

CoreとSDKの公開前に、対象commitをGitHubへpushした状態で`mise run verify full`を実行し、全品質ゲートの成功を確認してください。検証に成功したらcommitを切り替えず、`mise run release core`、`mise run release sdk`または`mise run release all`を実行します。リリースコマンドはバージョンタグをpushして公開Workflowを起動します。LWSリリースWorkflowはイメージ作成・公開とdigest検証付きパッケージ生成を行い、SDKリリースWorkflowは公開用成果物をbuildしてGitHub Packagesへ公開します。検証と公開の間にcommitを追加・切り替えないでください。

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

- [利用マニュアル](docs/LWS%20v0.1.12利用マニュアル.md)
- [設計概要](docs/LabWebSystem設計概要.md)
- [Infrastructure仕様書](docs/Infrastructure仕様書.md)
- [Backend仕様書](docs/Backend仕様書.md)
- [機能テストルール](docs/機能テストルール.md)
- [LICENSE](LICENSE)
