# LabWebSystem Infrastructure仕様書

## 1. 原則

- `lwsctl`はLWS本体のライフサイクル、Backendはアプリ管理とDocker操作、Docker Composeは実行環境のオーケストレーションを担当する。
- Backend SQLiteをアプリ定義・公開ルート・望ましい状態の正本とする。Caddyfile、Avahi用hosts、LWS用Compose overrideは派生物であり、手編集しない。
- 登録アプリの`compose.yaml`はアプリ自身の正本である。LWSは書き換えず、公開サービスへ最小のoverrideだけを追加する。
- 初期実装はLAN内HTTPだけを提供し、ホスト公開ポートは80/tcpと5353/udpだけとする。
- 認証・認可は持たない。LWSへ到達できるLAN内端末を管理者として扱い、HTTP通信上のsecretの秘密性は保証しない。
- LWS所有を確認できないDockerリソースは操作しない。

## 2. コンポーネントと実行技術

| 対象 | 責務・採用技術 |
| --- | --- |
| `lwsctl` | LWS本体Composeのライフサイクル。Docker socketは使わない |
| Backend | 状態の永続化、アプリ操作、検証、設定生成、LWS所有Docker操作。Goバイナリ、`git`、Docker CLI、Docker Compose pluginを含む最小Linuxイメージで実行する |
| Caddy | HTTPの唯一の公開入口。公式イメージを使用する |
| Avahi | 生成hostsにある`.local`名をmDNSで広告する。LWS公式イメージを使用する |
| 登録アプリ | アプリ自身のCompose構成で実行。Docker socketは使わない |

- BackendだけがDocker socketを使う。登録アプリへのsocket mountを禁止する。
- Backendは物理デバイスの検出だけのため、ホストの`/dev`と`/run/udev`をread-onlyでmountする。Backend自身へdeviceを割り当てず、実際のdevice割り当ては生成overrideを通じて登録アプリだけへ行う。
- Backend、Caddy、Avahiの実行イメージは、同じLWS release versionに揃えたtagと固定digestを使う。`latest`は使わない。
- Backendは`docker compose`と`git`を実行するため、distrolessイメージを使わない。
- Backendはホストポートを公開せず、Caddy経由でだけ到達可能にする。CORSは許可しない。状態変更にはJSONの`Content-Type`と許可済み`Host`を要求し、`Origin` headerがある要求は許可済みoriginだけを受け付ける。

## 3. Linux上の配置

| 区分 | パス | 所有者・用途 |
| --- | --- | --- |
| パッケージ | `/usr/bin/lwsctl`、`/usr/share/lws/` | APT/DNFが所有する。インストール・更新時に起動しない |
| 設定 | `/etc/lws/config.env`、`/etc/lws/secret.key` | LWSだけが読める。ホスト設定とsecret暗号化鍵 |
| 状態 | `/var/lib/lws/database.sqlite` | Backend SQLite |
| アプリ | Backendの`lws-backend-data` Named Volume内の`/var/lib/lws/apps/<app-id>/source/`、`runtime/` | 取得済みsourceと生成した`lws.override.yaml`、`app.env` |
| 生成物 | `/var/lib/lws/generated/Caddyfile`、`hosts` | SQLiteから生成する派生物 |

- アプリの永続データ、BackendのDB、取得済みsource、生成設定はDocker Named Volumeに保持する。登録アプリのComposeでhost bind mountおよび匿名Volumeを禁止するため、LWSはアプリ領域をホストへbind mountしない。`build.context`はBackendがNamed Volume内のsourceからDockerへ送信する。
- 通常の`lwsctl down`はLWS本体の実行環境だけを削除し、設定、LWS管理データ、LWS所有volumeとアプリ資源を保持する。`lwsctl down --recursive`は現在のinstallation IDとLWS所有labelで識別できるアプリcontainerとedge networkも削除する。`lwsctl down --purge`は確認後にLWS本体の設定、状態、永続volumeを削除し、`--recursive`指定時はアプリのLWS所有named volumeも削除する。別のDockerシステムの資源は削除しない。パッケージの削除はAPT / DNFへ委譲する。

## 4. ネットワーク、DNS、Reverse Proxy

| network | 接続先 | 用途 |
| --- | --- | --- |
| `lws-internal` | Backend、Caddy | LWS内部通信 |
| ホストネットワーク | Avahi | LAN mDNS受信・応答 |
| `lws-app-<app-id>-edge` | Caddy、指定された公開サービス | アプリごとの公開経路 |

- Backendはアプリごとにedge networkを作成し、LWS所有、installation ID、app-idのlabelを付ける。
- 公開サービスだけに`lws-<app-id>`というnetwork aliasを付ける。CaddyはCompose service名ではなく、このaliasとmanifestのportへ転送する。
- アプリは他アプリのedge networkおよび`lws-internal`へ接続しない。初期実装ではアプリ間通信を常に拒否する。
- Caddyのedge network接続はBackendがDocker APIで行い、Backend起動後はSQLiteの正本から再調整する。
- Docker daemonのアドレスプールは、想定アプリ数に必要なbridge network数を運用者が確保する。枯渇時は登録・起動を失敗させ、既存アプリを変更しない。
- `config.env`には`LWS_BASE_DOMAIN`、`LWS_VERSION`、`LWS_INSTALLATION_ID`、`LWS_PUBLIC_ADDRESS`を保存する。初回`lwsctl start --domain`はデフォルト経路のIPv4を保存し、`LWS_PUBLIC_ADDRESS`があれば優先する。IPを一意に決められなければ失敗する。
- Backendは起動時および状態変更後に、SQLiteの有効アプリ一覧からhostsとCaddyfileを一時ファイルへ生成しatomic renameする。hostsは`api-<base-domain>`と各アプリURLを`LWS_PUBLIC_ADDRESS`へ対応付ける。Dashboardも`dashboard-<base-domain>`のシステムアプリとして一覧に含める。Caddyfileは登録アプリと同じedge network上のDashboardへ転送し、Dashboardの`/api/v1`だけBackendへ中継して`api-<base-domain>`のHostを渡す。複数ラベルのbase domainは、末尾の`.local`より前をハイフンで連結して1ラベルにする。
- Dashboard imageはLWSリリース成果物として配布するが、基盤Compose serviceにはしない。Backendが起動時にシステムアプリとして状態表へ登録し、通常アプリと同じCompose runtime、edge network、Reverse Proxy経路で起動する。Dashboardは初期状態で起動し、他アプリと同様に開始・停止できる。システムアプリはAPIとDB制約で削除・登録解除できない。
- Avahiは生成hostsの更新後、所有確認済みコンテナを再起動して静的レコードを再読込する。Caddyは生成volume上のCaddyfileを検証後、所有確認済みコンテナのadmin APIへ`caddy reload`を実行して無停止再読込する。反映に失敗した場合は生成ファイルを復元する。
- Avahiはホストネットワーク上で動作し、UDP 5353で`.local`レコードをmDNS広告する。Backendが生成する`hosts`を静的ホスト定義として使い、ルーターDNSや通常のDNS問い合わせは扱わない。
- mDNSクライアントが有効な端末は同一LAN上で名前を解決できる。通常DNSのみに対応する端末や、mDNSを通さないVLANを越えた名前解決には対応しない。
- Backend起動時は、SQLiteで`ACTIVE`かつ`desired_state=RUNNING`のアプリについて、edge networkの存在とCaddy接続をDocker実状態と照合し、不足部分だけを再作成・再接続する。Caddy起動待ちの間は再試行し、DBの状態を先に変更しない。
- LWSサーバーIPは固定またはDHCP予約を推奨する。`lwsctl start`は80/tcpまたはUDP 5353の競合時に起動せず、既存のmDNSサービスを停止・変更しない。

## 5. 登録アプリ契約

### 5.1 アプリsource

GitHubリポジトリまたはZIPにComposeファイルが必要である。`lws.manifest.yaml`は任意であり、表示情報と公開先の初期値を提供する。ZIPは直下または直下の単一フォルダをアプリrootとする。

```text
compose.yaml
lws.manifest.yaml
```

メインComposeとoverrideは利用者が明示した順に読み込む。暗黙のoverrideは適用しない。

### 5.2 `lws.manifest.yaml`

- rootにある通常ファイル、UTF-8、8 KiB以下、単一YAML documentとする。symlink、anchor、alias、独自tag、duplicate key、暗黙型変換、未定義キーを拒否する。
- v1の完全schemaは次のとおりとする。

```yaml
apiVersion: lws/v1
composeFile: compose.yaml
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
| `composeFile` | 任意。source rootにあるComposeファイル名。明示選択がない場合に使用する |
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

- overrideは選択された公開サービスへapp固有edge network、`lws-<app-id>` alias、LWS所有labelを追加する。検証済みの全named volumeにもLWS所有labelを追加する。それ以外のservice、image、build、command、environment、依存関係、volume設定値を変更しない。
- `docker compose config --format json`の前に、source treeのsymlink、`include`、`extends`、`env_file`、`label_file`、`volumes_from`、ファイル型`configs`/`secrets`、`build.additional_contexts`、リモートGit build contextを拒否する。
- `build.context`と`build.dockerfile`は、実体パスがsource root配下の場合だけ許可する。絶対パス、root外へ解決される相対パス、external network、external volumeを拒否する。パスを丸めたりComposeを書き換えたりして受理しない。
- 事前検査を通過したものだけを`docker compose config --format json`で正規化し、選択したserviceの存在を確認する。公開serviceやportは推測しない。
- 初回登録はsourceと、存在する場合はmanifestの検証後に`CONFIGURING`へ遷移し、containerを起動しない。Dashboardまたは設定APIで必須環境変数とdevice bindingを保存した後の開始操作だけがComposeを起動する。
- source検証と実効Compose検証の両方で、bind mount、匿名volume、tmpfs、host port、host network/PID/IPC、privileged、Docker socket、named volume以外のmountを拒否する。`devices`は例外であり、Composeのcontainer側targetと権限を保ちつつ、LWSデバイスプールから解決した現在の`/dev` pathだけを生成overrideへ渡す。開発時のファイル同期が必要なアプリは、プロジェクトrootからの相対pathだけを指定したDocker Compose Watchを利用する。

## 7. ライフサイクルと禁止事項

- `lwsctl start`は設定を確認してLWS本体Composeを起動する。ベースドメインは`.local`で終わるhostnameを指定する。旧ベースドメインからの移行は`lwsctl start --domain <名前>.local --force`で行う。`stop`は実行中のLWS本体Composeを停止し、`-r`または`--recursive`指定時はLWS所有・installation ID一致のアプリcontainerも停止する。`down`はLWS本体Composeのコンテナとnetworkを削除し、`--recursive`指定時はアプリcontainerとedge networkも削除する。`rebuild`は本体と派生設定を再構成するが、アプリsourceやvolumeを削除しない。
- 通常の停止・同期・再構成で`docker compose down --volumes`を使わない。完全削除時だけ、project名、LWS所有label、installation ID、app-idが一致するvolumeを削除できる。
- `docker system prune`、`docker volume prune`、未検証Composeの起動、host bind mountまたは匿名Volumeの許可、`compose.override.yaml`の暗黙取込み、Backend以外へのDocker socket付与を禁止する。
