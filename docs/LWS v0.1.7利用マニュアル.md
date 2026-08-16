# LWS v0.1.7 利用マニュアル

## LWSでできること

LWSは、研究室や家庭内LANで使うWebアプリを、決まったURLで公開・管理するための仕組みです。

LWSを使うと、次のことができます。

- WebアプリをGitHubリポジトリから登録する
- アプリをDocker Composeで起動する
- アプリごとに専用のURLを割り当てる
- アプリを停止・再起動・再構成する
- アプリの設定値やsecretを管理する
- アプリのログを確認する
- アプリを一時的に登録解除し、後で復元する
- アプリとその保存データを確認付きで完全に削除する
- LWS本体を停止・更新・再構成する

## 公開されるURL

LWSの起動時にベースドメインを一つ設定します。

```text
example.internal
```

登録したアプリにはサブドメインを割り当てます。例えば`予約システム`に`reserve`を割り当てると、次のURLでアクセスできます。

```text
reserve.example.internal
```

URLの名前解決はLWSのDNS、Webアクセスの振り分けはLWSのReverse Proxyが担当します。利用する端末がLWSのDNSを使うように、LAN側のDHCPやDNS設定を整えてください。

## 利用を始める

### 1. LWSをインストールする

Ubuntu系では`.deb`、AlmaLinux系では`.rpm`を使ってインストールします。インストールはLWSのファイルを配置するだけで、LWSは自動起動しません。

### 2. LWSを初回起動する

```sh
sudo lwsctl start --domain example.internal
```

初回起動時に、ベースドメインとLWSの実行環境が準備されます。

起動前に、ホストの80番ポートと53番ポート（TCP/UDP）が空いている必要があります。

### 3. 状態を確認する

```sh
sudo lwsctl status
```

設定済みのベースドメインと、LWSの各コンテナの状態を確認できます。

## LWS本体を操作する

| コマンド | できること |
| --- | --- |
| `lwsctl start` | LWSを起動する。初回はベースドメインを設定する |
| `lwsctl stop` | LWSを停止する。設定や保存データは残る |
| `lwsctl status` | 設定と実行状態を確認する |
| `lwsctl rebuild` | LWSの設定を再生成して実行環境を作り直す |
| `lwsctl update` | LWSのパッケージとDockerイメージを更新する |
| `lwsctl down` | LWSの実行環境を削除する。設定や保存データは残る |

`down`は実行環境だけを削除します。設定・状態・保存データまで削除する場合は、確認のうえで次を実行します。

```sh
sudo lwsctl down --purge
```

確認を省略する場合だけ`--force`を追加します。

```sh
sudo lwsctl down --purge --force
```

## アプリを登録する

登録するアプリのGitHubリポジトリには、次の2ファイルが必要です。

```text
compose.yaml
lws.manifest.yaml
```

`lws.manifest.yaml`には、アプリの名前と、外部公開するCompose service・portを記載します。

```yaml
apiVersion: lws/v1
metadata:
  name: 研究室予約システム
  description: 研究室の設備予約を管理するWebアプリケーション
public:
  service: web
  port: 3000
```

Backend APIで、GitHubリポジトリ、ブランチまたはref、公開subdomainを指定して登録します。登録後、LWSがリポジトリを取得・検査し、問題がなければDocker Composeでアプリを起動します。

アプリの公開serviceやportは推測されません。manifestに記載したserviceがCompose内に存在しない場合、アプリは起動されません。

## アプリを管理する

登録後は、アプリごとに次の操作ができます。

- 現在の状態を確認する
- 起動・停止する
- Composeを再構成する
- リポジトリのrefや公開subdomainを変更する
- 最新のsourceを再取得して同期する
- コンテナログを確認する
- 環境変数を設定する

アプリの変更処理はすぐに受付結果を返し、バックグラウンドで実行されます。処理の進み具合はOperationの状態として確認できます。

同じアプリに複数の変更を同時に行うことはできません。前の処理が完了してから次の操作を行ってください。

## 設定値とsecret

アプリごとに環境変数を設定できます。パスワードやAPIキーなどsecretとして登録した値は暗号化して保存され、取得時に値そのものは表示されません。

ただし、LWS v0.1.7にはAPIの認証・認可がありません。LWSへ到達できるLAN内端末を管理者として扱うため、信頼できるネットワーク内で使用してください。

## 登録解除と完全削除

### 登録解除

登録解除では、アプリの実行を停止して公開URLから外します。一方で、次のデータは保持されます。

- アプリのsource
- アプリの設定
- Dockerの保存データ
- LWS内のアプリ記録

再登録すれば、保持した情報を使って復元できます。

### 完全削除

完全削除は、登録解除済みのアプリに対してだけ実行できます。アプリのsource、設定、保存データ、LWS内の記録を削除します。

この操作は元に戻せないため、確認が必要です。

## v0.1.7での注意点

- 管理操作の中心はBackend APIです。
- Dashboardはまだ実用的な管理画面ではありません。
- TypeScript SDKはまだモック段階です。
- APIの認証・認可はありません。
- WebSocketは使用せず、処理状態やログの通知にはSSEを使います。
- LWSのDNSをLAN端末へ配布するDHCP設定は、利用者側で行います。
- 登録アプリはDocker Compose形式で用意する必要があります。

## 困ったとき

まずLWS本体の状態を確認します。

```sh
sudo lwsctl status
```

設定を再生成して起動環境を作り直す場合は、次を実行します。

```sh
sudo lwsctl rebuild
```

LWSを停止してから再度起動する場合は、次の順で実行します。

```sh
sudo lwsctl stop
sudo lwsctl start
```

アプリだけを操作する場合は、DashboardまたはBackend APIから対象アプリのOperationとログを確認してください。
