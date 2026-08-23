# LabWebSystem

LabWebSystem（LWS）は、研究室や家庭内LANでWebアプリを公開・管理するための仕組みです。

アプリはGitHubリポジトリから登録します。LWSがDocker Composeでアプリを動かし、アプリごとのURLをLAN内へ公開します。

> 現在のLWSは v0.1.7 です。CLIとBackendによるアプリ管理の基盤を使えます。DashboardとTypeScript SDKは、まだ開発中です。

## できること

- GitHubリポジトリにあるWebアプリを登録する
- Docker Composeでアプリを起動・停止・更新する
- `app-name.example.internal`の形でアプリごとのURLを公開する
- アプリの設定値、secret、ログ、実行状態を管理する
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

詳しい操作、アプリの登録方法、完全削除については、[利用マニュアル](docs/LWS%20v0.1.7利用マニュアル.md)を参照してください。

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

LWSはリポジトリを取得して内容を確認し、問題がなければアプリを起動します。アプリの登録・設定・操作はBackend APIから行います。APIの詳しい使い方は[利用マニュアル](docs/LWS%20v0.1.7利用マニュアル.md)を参照してください。

## 開発

このリポジトリでは、開発に必要なツールとコマンドを`mise`で管理します。

```sh
mise install
mise run verify          # 普段の開発・通常CI用。約数秒
mise run verify qa       # DockerとRobot QAを含む確認
mise run verify release  # リリース前の確認とパッケージ生成
mise run dev             # 開発用Composeを起動
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
| `dashboard/` | Dashboardのコンテナ定義 |
| `sdk/` | 外部アプリ向けTypeScript SDK |
| `infrastructure/` | LWS本体のDocker Compose、DNS、Reverse Proxy設定 |
| `packaging/` | `.deb`・`.rpm`パッケージ定義 |
| `qa/` | Robot Frameworkの受け入れテスト |

## 注意点

- 管理APIには、まだ認証・認可がありません。信頼できるLAN内だけで使ってください。
- DashboardとTypeScript SDKは、実用機能の実装途中です。
- LAN端末へLWSのDNSを配るDHCP・DNS設定は、利用するネットワーク側で行います。

## 関連資料

- [利用マニュアル](docs/LWS%20v0.1.7利用マニュアル.md)
- [設計概要](docs/LabWebSystem設計概要.md)
- [Infrastructure仕様書](docs/Infrastructure仕様書.md)
- [Backend仕様書](docs/Backend仕様書.md)
- [機能テストルール](docs/機能テストルール.md)
- [LICENSE](LICENSE)
