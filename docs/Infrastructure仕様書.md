# LabWebSystem Infrastructure仕様書

## 1. 原則

- `lwsctl`はLWS本体のライフサイクル、Backendはアプリ管理とDocker操作、Docker Composeは実行環境のオーケストレーションを担当する。
- Backend SQLiteをアプリ定義・公開ルート・望ましい状態の正本とする。Caddyfile、CoreDNS hosts、LWS用Compose overrideは派生物であり、手編集しない。
- 登録アプリの`compose.yaml`はアプリ自身の正本である。LWSは書き換えず、公開サービスへ最小のoverrideだけを追加する。
- 初期実装はLAN内HTTPだけを提供し、ホスト公開ポートは80/tcp、53/tcp、53/udpだけとする。
- 認証・認可は持たない。LWSへ到達できるLAN内端末を管理者として扱い、HTTP通信上のsecretの秘密性は保証しない。
- LWS所有を確認できないDockerリソースは操作しない。

## 2. コンポーネントと実行技術

| 対象 | 責務・採用技術 |
| --- | --- |
| `lwsctl` | LWS本体Composeのライフサイクル。Docker socketは使わない |
| Backend | 状態の永続化、アプリ操作、検証、設定生成、LWS所有Docker操作。Goバイナリ、`git`、Docker CLI、Docker Compose pluginを含む最小Linuxイメージで実行する |
| Caddy | HTTPの唯一の公開入口。公式イメージを使用する |
| CoreDNS | LWSベースドメインの名前解決と外部問い合わせの転送。公式イメージを使用する |
| 登録アプリ | アプリ自身のCompose構成で実行。Docker socketは使わない |

- BackendだけがDocker socketを使う。登録アプリへのsocket mountを禁止する。
- Backend、Caddy、CoreDNSの実行イメージは、同じLWS release versionに揃えたtagと固定digestを使う。`latest`は使わない。
- Backendは`docker compose`と`git`を実行するため、distrolessイメージを使わない。
- Backendはホストポートを公開せず、Caddy経由でだけ到達可能にする。CORSは許可しない。状態変更にはJSONの`Content-Type`と許可済み`Host`を要求し、`Origin` headerがある要求は許可済みoriginだけを受け付ける。

## 3. Linux上の配置

| 区分 | パス | 所有者・用途 |
| --- | --- | --- |
| パッケージ | `/usr/bin/lwsctl`、`/usr/share/lws/` | APT/DNFが所有する。インストール・更新時に起動しない |
| 設定 | `/etc/lws/config.env`、`/etc/lws/secret.key` | LWSだけが読める。ホスト設定とsecret暗号化鍵 |
| 状態 | `/var/lib/lws/database.sqlite` | Backend SQLite |
| アプリ | `/var/lib/lws/apps/<app-id>/source/`、`runtime/` | 取得済みsourceと生成した`lws.override.yaml`、`app.env` |
| 生成物 | `/var/lib/lws/generated/Caddyfile`、`hosts` | SQLiteから生成する派生物 |

- アプリ永続データはhost bind mountではなくDocker named volumeだけに置く。
- 通常の`lwsctl down`は設定、LWS管理データ、LWS所有volumeを保持する。`lwsctl down --purge`だけが確認後に削除する。パッケージの削除はAPT / DNFへ委譲する。

## 4. ネットワーク、DNS、Reverse Proxy

| network | 接続先 | 用途 |
| --- | --- | --- |
| `lws-internal` | Backend、Caddy、CoreDNS | LWS内部通信 |
| `lws-app-<app-id>-edge` | Caddy、指定された公開サービス | アプリごとの公開経路 |

- Backendはアプリごとにedge networkを作成し、LWS所有、installation ID、app-idのlabelを付ける。
- 公開サービスだけに`lws-<app-id>`というnetwork aliasを付ける。CaddyはCompose service名ではなく、このaliasとmanifestのportへ転送する。
- アプリは他アプリのedge networkおよび`lws-internal`へ接続しない。初期実装ではアプリ間通信を常に拒否する。
- Caddyのedge network接続はBackendがDocker APIで行い、Backend起動後はSQLiteの正本から再調整する。
- Docker daemonのアドレスプールは、想定アプリ数に必要なbridge network数を運用者が確保する。枯渇時は登録・起動を失敗させ、既存アプリを変更しない。
- `config.env`には`LWS_BASE_DOMAIN`、`LWS_VERSION`、`LWS_INSTALLATION_ID`、`LWS_PUBLIC_ADDRESS`を保存する。初回`lwsctl start --domain`はデフォルト経路のIPv4を保存し、`LWS_PUBLIC_ADDRESS`があれば優先する。IPを一意に決められなければ失敗する。
- Backendは起動時および状態変更後に、SQLiteの有効アプリ一覧からhostsとCaddyfileを一時ファイルへ生成しatomic renameする。hostsは`api.<base-domain>`と各アプリURLを`LWS_PUBLIC_ADDRESS`へ対応付ける。CaddyfileにはAPIのReverse Proxyとmanifestの公開service・portを反映する。
- CoreDNSはhosts再読込で反映する。Caddyは生成volume上のCaddyfileを検証後、所有確認済みコンテナのadmin APIへ`caddy reload`を実行して無停止再読込する。いずれかの反映に失敗した場合は既存設定を維持する。
- LWSをDNSとして配布するDHCP設定は運用者の責務とする。LWSサーバーIPは固定またはDHCP予約を推奨する。`lwsctl start`は53/tcp・53/udp競合時に起動しない。

## 5. 登録アプリ契約

### 5.1 必須ファイル

リポジトリrootには、通常のComposeアプリとしての`compose.yaml`とLWS連携情報の`lws.manifest.yaml`が必要である。

```text
compose.yaml
lws.manifest.yaml
```

`compose.override.yaml`はローカル開発用に使用できるが、LWSは明示的に`-f compose.yaml`を指定するため読まない。

### 5.2 `lws.manifest.yaml`

- rootにある通常ファイル、UTF-8、8 KiB以下、単一YAML documentとする。symlink、anchor、alias、独自tag、duplicate key、暗黙型変換、未定義キーを拒否する。
- v1の完全schemaは次のとおりとする。

```yaml
apiVersion: lws/v1
metadata:
  name: 研究室予約システム
  description: 研究室の設備予約を管理するWebアプリケーション
public:
  service: web
  port: 3000
```

| 項目 | 要件 |
| --- | --- |
| `apiVersion` | 文字列`lws/v1`のみ |
| `metadata` | `name`と`description`だけを持つobject |
| `metadata.name` | 前後空白なし、1〜80文字のUnicode文字列 |
| `metadata.description` | 0〜500文字のプレーンテキスト |
| `public` | `service`と`port`だけを持つobject |
| `public.service` | `^[a-z][a-z0-9_-]{0,62}$` |
| `public.port` | 1〜65535の整数 |

manifestの表示情報はComposeのトップレベル`name`と無関係である。公開subdomainは登録時にBackendへ与え、manifestには書かない。

## 6. Composeの実行と検証

```text
docker compose --project-name lws-app-<app-id> \
  --env-file /var/lib/lws/apps/<app-id>/runtime/app.env \
  -f /var/lib/lws/apps/<app-id>/source/compose.yaml \
  -f /var/lib/lws/apps/<app-id>/runtime/lws.override.yaml up -d
```

- overrideはmanifest指定サービスへapp固有edge network、`lws-<app-id>` alias、LWS所有labelを追加する。検証済みの全named volumeにもLWS所有labelを追加する。それ以外のservice、image、build、command、environment、依存関係、volume設定値を変更しない。
- `docker compose config --format json`の前に、source treeのsymlink、`include`、`extends`、`env_file`、`label_file`、`volumes_from`、ファイル型`configs`/`secrets`、`build.additional_contexts`、リモートGit build contextを拒否する。
- `build.context`と`build.dockerfile`は、実体パスがsource root配下の場合だけ許可する。絶対パス、root外へ解決される相対パス、external network、external volumeを拒否する。パスを丸めたりComposeを書き換えたりして受理しない。
- 事前検査を通過したものだけを`docker compose config --format json`で正規化し、manifest指定serviceの存在を確認する。公開serviceやportは推測しない。
- 実効Composeではbind mount、匿名volume、tmpfs、host port、host network/PID/IPC、privileged、device、Docker socket、named volume以外のmountを拒否する。

## 7. ライフサイクルと禁止事項

- `lwsctl start`は設定を確認してLWS本体Composeを起動する。`stop`は実行中のLWS本体Composeを停止する。`down`はLWS本体Composeのコンテナとnetworkを削除する。`rebuild`は本体と派生設定を再構成するが、アプリsourceやvolumeを削除しない。
- 通常の停止・同期・再構成で`docker compose down --volumes`を使わない。完全削除時だけ、project名、LWS所有label、installation ID、app-idが一致するvolumeを削除できる。
- `docker system prune`、`docker volume prune`、未検証Composeの起動、外部リポジトリ由来bind mountの許可、`compose.override.yaml`の暗黙取込み、Backend以外へのDocker socket付与を禁止する。
