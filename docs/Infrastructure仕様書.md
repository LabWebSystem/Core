# LabWebSystem Infrastructure仕様書

## 1. 原則

LWS固有のライフサイクルは`lwsctl`、アプリ管理とDocker操作はBackend、実行環境のオーケストレーションはDocker Composeが担当する。

- アプリ定義、公開ルート、望ましい状態の正本はBackendのSQLiteである。
- アプリの`compose.yaml`はアプリ自身のCompose構成の正本である。LWSはこれを変換・書換えしない。
- LWSは`lws.manifest.yaml`で指定された公開サービスへ、最小のCompose overrideだけを追加する。
- 実効Compose、Caddyfile、CoreDNS用hostsは派生物であり、手編集しない。
- LWS所有を確認できないDockerリソースは操作しない。
- 初期実装はLAN内HTTPだけを提供する。ホストへ公開するポートは80/tcp、53/tcp、53/udpだけである。
- 初期実装は利用者の認証・認可を持たない。LWSへ到達できるLAN内の端末は、すべてLWSの管理者として扱う。HTTP通信の秘密性は保証しない。

## 2. コンポーネント

| コンポーネント | 責務 | Docker socket |
| --- | --- | --- |
| `lwsctl` | LWS本体Composeの開始、停止、状態表示、再構成、更新、削除 | 使用しない |
| Backend | 状態の永続化、アプリ操作、Compose検証、設定生成、LWS所有Dockerリソースの操作 | 使用する |
| Caddy | HTTPの唯一の公開入口 | 使用しない |
| CoreDNS | LWSベースドメインの名前解決と、それ以外の問い合わせの転送 | 使用しない |
| 登録アプリ | アプリ自身のCompose構成による実行 | 使用しない |

Docker socketはDocker daemonを完全に操作できる強い権限である。Backend以外、とりわけ登録アプリへDocker socketをmountしてはならない。Backendは未信頼な入力から任意のDocker API要求を組み立てない。

Backendはホストポートを公開せず、管理HTTP APIはCaddy経由でだけ到達可能にする。CaddyはCORSを許可せず、状態変更APIにJSONの`Content-Type`と許可済み`Host`を要求する。`Origin` headerがあるブラウザ要求は許可済みoriginだけを受け付ける。これはLAN上の直接的な管理操作を制限する認証機構ではなく、別originのWebページがブラウザを踏み台に状態変更することを防ぐための入力検査である。

### 2.1 実行技術

| 対象 | 採用技術 | 方針 |
| --- | --- | --- |
| LWS本体の実行基盤 | Docker Compose | LWS本体と登録アプリの正規オーケストレーター |
| Backend実行イメージ | Goバイナリを含む最小Linuxイメージ | `git`、Docker CLI、Docker Compose pluginを同梱する。BackendはDocker socketだけを利用する |
| HTTP Reverse Proxy | Caddy公式イメージ | CaddyfileからHTTPを公開する |
| DNS | CoreDNS公式イメージ | hosts pluginとforwardで名前解決する |
| イメージ識別 | release versionとdigest | リリースごとにLWS本体イメージを同一versionへ揃え、実行用Composeではtagと一致するdigestを指定する |

Backend、Caddy、CoreDNSの実行イメージは、公式またはLWSが所有するGHCRイメージを使用し、digestを固定する。`latest`タグは使用しない。BackendイメージはDocker Composeの仕様どおりCLIを実行するため、distrolessイメージを使用しない。

## 3. Linux上の配置

### 3.1 パッケージ管理下

```text
/usr/bin/lwsctl
/usr/share/lws/compose.yaml
/usr/share/lws/version
/usr/share/lws/install.sh
```

上記はパッケージの所有物である。パッケージのインストールと更新はファイル配置だけを行い、LWSを起動しない。

### 3.2 設定とLWS管理データ

```text
/etc/lws/
├── config.env
└── secret.key

/var/lib/lws/
├── database.sqlite
├── apps/
│   └── <app-id>/
│       ├── source/
│       └── runtime/
│           ├── lws.override.yaml
│           └── app.env
└── generated/
    ├── Caddyfile
    └── hosts
```

`/etc/lws/config.env`はホスト固有設定、`/etc/lws/secret.key`はsecret暗号化用のランダム鍵であり、いずれもLWSだけが読める権限にする。`/var/lib/lws`はBackendのDB、取得済みsource、LWS生成物を置く。

アプリの永続データは`/var/lib/lws`へbind mountしない。登録アプリの名前付きvolumeはDockerが管理し、Docker Composeが自動付与するprojectラベルと固定project名で管理対象を識別する。通常の`lwsctl uninstall`は設定、LWS管理データ、LWS所有volumeを保持し、`lwsctl uninstall --purge`だけが確認後に削除する。

## 4. LWS本体Compose

LWS本体のCompose project名は`lws`で固定する。ComposeはBackend、Caddy、CoreDNSを含み、すべてに次のラベルを付ける。

```text
com.labwebsystem.owner=lws
com.labwebsystem.installation=<installation-id>
com.labwebsystem.role=<backend|proxy|dns>
```

| network | 接続先 | 用途 |
| --- | --- | --- |
| `lws-internal` | Backend、Caddy、CoreDNS | LWS内部通信 |

登録アプリごとに、Backendはbridge networkの`lws-app-<app-id>-edge`を作成する。このnetworkにはCaddyとmanifest指定の公開サービスだけを接続する。CaddyはBackendがDocker APIで実行中コンテナへ接続し、LWS本体の再起動後はBackendがSQLiteの正本から接続状態を再調整する。各networkにはLWS所有、installation ID、app-idを示すラベルを付ける。

公開サービスには、そのアプリのedge network内だけで有効な`lws-<app-id>`というaliasを付与する。CaddyはComposeのservice名ではなく、このaliasとmanifestのportをupstreamに使用する。したがって、複数のアプリが同じ`web`というservice名を使っても衝突しない。

アプリは他アプリのedge networkへ接続しないため、登録アプリ間の直接通信は初期実装では常に拒否される。登録アプリは`lws-internal`へも接続してはならない。LWS本体内の固定bind mountは、`/etc/lws`および`/var/lib/lws`のLWS専用パスだけに許可する。これは外部リポジトリのComposeでは指定できない例外である。

## 5. ホスト設定、DNS、Reverse Proxy

`config.env`には少なくとも`LWS_BASE_DOMAIN`、`LWS_VERSION`、`LWS_INSTALLATION_ID`、`LWS_PUBLIC_ADDRESS`を保存する。初回の`lwsctl start --domain`はデフォルト経路のIPv4を保存し、`LWS_PUBLIC_ADDRESS`環境変数がある場合はそれを優先する。IPv4を一意に決定できない場合は起動を失敗させる。

BackendはSQLiteの有効なアプリ一覧から、`/var/lib/lws/generated/hosts`と`Caddyfile`を一時ファイルとatomic renameで生成する。

- hostsには`api.<base-domain>`と各`<subdomain>.<base-domain>`を`LWS_PUBLIC_ADDRESS`へ対応付ける。
- CoreDNSはhostsファイルの再読込を使用する。BackendはDNSプロトコルを実装せず、CoreDNSを再起動して反映しない。
- Caddyは80/tcpを公開する唯一のコンテナである。BackendはCaddyfileを検証してから置き換え、所有ラベルとinstallation IDが一致するCaddyだけへ`SIGUSR1`を送って無停止再読込する。
- LWSをDNSとして配布するDHCP設定は運用者の責務とする。LWSサーバーのIPは固定またはDHCP予約を推奨する。`lwsctl start`は53/tcpと53/udpの競合を起動前に検査し、競合時は起動しない。
- Docker daemonのアドレスプールは、想定アプリ数に対して十分な数のbridge networkを作成できるよう、運用者が設定する。network作成時にアドレス範囲が不足した場合、Backendは登録または起動を失敗させ、既存アプリを変更しない。

検証または再読込に失敗した場合、既存の有効な設定を維持する。

## 6. 登録アプリのCompose契約

### 6.1 リポジトリ構成

LWSが受理するリポジトリrootには、通常のDocker Composeアプリとしての`compose.yaml`と、LWS連携情報である`lws.manifest.yaml`が必要である。

```text
compose.yaml
lws.manifest.yaml
```

`compose.override.yaml`は任意のローカル開発用ファイルとして使用できる。LWSは必ず`-f compose.yaml`を明示指定するため、`compose.override.yaml`を読まない。

### 6.2 manifest

`lws.manifest.yaml`はリポジトリrootにある通常ファイルでなければならず、symlinkを許可しない。UTF-8、8 KiB以下、単一YAML documentとし、anchor、alias、独自tag、duplicate key、未定義キー、型の暗黙変換を拒否する。

v1の完全なschemaは次である。

```yaml
apiVersion: lws/v1

metadata:
  name: 研究室予約システム
  description: 研究室の設備予約を管理するWebアプリケーション

public:
  service: web
  port: 3000
```

| パス | 要件 |
| --- | --- |
| `apiVersion` | 文字列`lws/v1`のみ |
| `metadata` | `name`と`description`だけを持つobject |
| `metadata.name` | 1〜80文字の前後空白を含まないUnicode文字列 |
| `metadata.description` | 0〜500文字のプレーンテキスト |
| `public` | `service`と`port`だけを持つobject |
| `public.service` | `^[a-z][a-z0-9_-]{0,62}$`に一致する文字列 |
| `public.port` | 1〜65535の整数 |

manifestの表示名はComposeのトップレベル`name`と無関係である。LWSは`--project-name lws-app-<app-id>`を明示する。サブドメインは登録時にBackendへ与える公開識別子であり、manifestには含めない。

### 6.3 起動方法と安全性

Backendは次の概念のコマンドでアプリを起動する。

```text
docker compose \
  --project-name lws-app-<app-id> \
  --env-file /var/lib/lws/apps/<app-id>/runtime/app.env \
  -f /var/lib/lws/apps/<app-id>/source/compose.yaml \
  -f /var/lib/lws/apps/<app-id>/runtime/lws.override.yaml \
  up -d
```

`lws.override.yaml`はBackendが生成する最小のoverrideである。manifestの`public.service`にだけ、事前作成した`lws-app-<app-id>-edge`への接続、`lws-<app-id>`のnetwork alias、固定のLWS所有ラベルを追加する。実効Composeの検証後、宣言済みnamed volumeすべてにも固定のLWS所有ラベルを追加する。アプリのservice、image、build、command、environment、依存関係、named volumeの設定値を変更しない。

起動前にBackendは、`docker compose config`を実行する前にsourceを事前検査する。これはComposeの意味を再実装せず、未信頼なComposeが設定展開中にプロジェクト外のファイルを読むことを防ぐための限定検査である。検査を通過した後だけ、上記二つのComposeファイルと`app.env`から`docker compose config --format json`で実効Composeを正規化して検証する。公開サービスを推測せず、manifestの`public.service`が実効Composeに存在することだけを確認する。

事前検査では、source tree内のsymlinkを拒否する。`include`、`extends`、`env_file`、`label_file`、`volumes_from`、ファイル型`configs`または`secrets`、`build.additional_contexts`、リモートGit build contextを拒否する。`build.context`と`build.dockerfile`だけは、実体パスがsource root配下である場合に許可する。絶対パス、source root外へ解決される相対パス、LWSが生成しないexternal networkまたはexternal volumeを拒否する。拒否時にパスを別の位置へ丸めたり、Composeを書き換えたりしない。

実効Composeでは次を拒否する。

- bind mount、匿名volume、tmpfs、およびnamed volume以外のすべてのmount
- host port、host network、host PID、host IPC、privileged、device、Docker socket
- external network、external volume、LWS管理外のhost path
- source root外を参照するbuild context、Dockerfile、env file、config、secret

アプリの永続データはアプリ自身が`compose.yaml`で宣言するnamed volumeにだけ置く。通常の停止、同期、再構成では`docker compose down --volumes`を使わない。完全削除時だけ、project名、LWS所有ラベル、installation ID、app-idが一致するvolumeを削除できる。

## 7. ライフサイクルと禁止事項

`lwsctl start`は設定を確認してLWS本体Composeを起動する。Backendの起動時とベースドメイン変更後は、SQLiteの正本からCaddyfileとhostsを再生成する。`lwsctl rebuild`はLWS本体を再構成して派生設定を再生成するが、アプリsourceやvolumeを削除しない。

次を禁止する。

- `docker system prune`、`docker volume prune`などの広範囲削除
- 未検証Composeの起動
- 外部リポジトリ由来のbind mountの許可または書換え
- `compose.override.yaml`のLWS実行への暗黙取込み
- Backend以外へのDocker socketの付与
- パッケージインストール時のコンテナ起動
