# Infrastructure・Backend シナリオテスト計画

## 1. 目的と使い方

本書は、[Infrastructure仕様書](Infrastructure仕様書.md)および[Backend仕様書](Backend仕様書.md)に基づき、InfrastructureとBackendの初期実装で満たす利用者シナリオと自動テストを整理する。

- 仕様の正本は上記の仕様書であり、本書は仕様を置き換えない。
- 各実装は、対応するシナリオとテスト項目を満たすことを受け入れ条件とする。
- `P0`は実装開始時に必須、`P1`は該当機能を実装する変更で必須、`P2`は代表的な統合・運用確認とする。
- 単体テストではDocker CLI・Docker APIをfakeに置き換える。実Dockerを必要とする代表経路だけを統合テストで確認する。

## 2. 正常系ユーザーシナリオ

| ID | 利用者の操作 | 期待結果 | 優先度 |
| --- | --- | --- | --- |
| N-01 | 管理者が`lwsctl start --domain example.internal`を初回実行する | 設定、installation ID、secret鍵、公開IPを用意し、LWS本体を起動する。`api.example.internal`を含むhostsが生成される。 | P0 |
| N-02 | 管理者がDashboardから適合するGitHubリポジトリをsubdomain付きで登録する | 登録要求を永続化してOperationを即時返却する。検証、取得、生成、起動後に`<subdomain>.<base-domain>`で公開される。 | P0 |
| N-03 | 管理者がアプリに必要な環境変数を設定して開始する | Backendが値を保存し、起動直前にのみ`app.env`を生成する。secret値は取得API、ログ、Operationに含まれない。 | P0 |
| N-04 | 同じCompose service名を持つ二つのアプリを登録する | appごとのedge networkと`lws-<app-id>` aliasにより、公開先が衝突せず、アプリ間通信もできない。 | P0 |
| N-05 | 管理者がアプリのrefを更新し`:sync`を実行する | 新sourceを一時領域で検証し、成功時だけsourceと派生物を原子的に切り替える。 | P0 |
| N-06 | 管理者がアプリを停止、開始、再構成する | 対象アプリだけを操作し、named volumeを保持する。状態はOperationとApplicationで観測できる。 | P1 |
| N-07 | 管理者がアプリを登録解除する | Caddyから切断後、コンテナとedge networkを削除する。source、runtime、設定、所有確認済みnamed volume、UNREGISTERED記録は保持する。 | P0 |
| N-08 | 管理者が登録解除済みアプリを完全削除する | 確認済み要求で、source、runtime、アプリデータ、所有情報が一致するnamed volume、DB記録を削除する。 | P0 |
| N-11 | 管理者が登録解除済みアプリを再登録する | 保持されたsource、runtime、設定を再利用し、初回登録と同じ入力なしでACTIVEへ復帰する。 | P1 |
| N-09 | Backendを再起動する | SQLiteの正本からCaddyのedge network接続と派生設定を再調整する。 | P1 |
| N-10 | 管理者がOperationまたはコンテナログを購読する | SSEで更新を受け取る。切断後はHTTP再取得および再接続で状態を回復できる。 | P1 |

## 3. 異常系ユーザーシナリオ

異常時の共通原則は、未検証の入力を実行しないこと、既存の稼働構成を不必要に変更しないこと、LWS所有を確認できないDocker資源を操作しないこと、secretとホスト絶対pathを外部へ出さないことである。

| ID | 異常条件または操作 | 期待結果 | 優先度 |
| --- | --- | --- | --- |
| E-01 | `start`に不正なbase domainを与える | 設定を作成・変更せず、日本語のエラーで失敗する。 | P0 |
| E-02 | 公開IPv4を一意に決定できない、または80/tcp・53/tcp・53/udpが競合する | LWS本体を起動せず、既存プロセスと無関係なDocker資源に触れない。 | P0 |
| E-03 | config.envまたはsecret鍵が欠損、破損、権限不正である | 安全に失敗する。secret内容を表示しない。 | P1 |
| E-04 | GitHub以外のURL、SSH URL、認証情報付きURL、query・fragment付きURL、ローカルpathを登録する | HTTP 400、`INVALID_ARGUMENT`。cloneを実行しない。 | P0 |
| E-05 | clone失敗、ref不存在、GitHub接続失敗、取得先の認可失敗が起きる | Operationを`failed`にする。既存source、稼働アプリ、公開設定を維持する。 | P0 |
| E-06 | manifestが存在しない、UTF-8でない、8 KiB超過、symlink、複数documentである | 登録を拒否し、sourceを採用しない。 | P0 |
| E-07 | manifestにanchor、alias、独自tag、duplicate key、未定義キー、型不正がある | 登録を拒否する。暗黙型変換で受理しない。 | P0 |
| E-08 | source treeにsymlink、またはComposeに`include`、`extends`、`env_file`等の外部読込機能がある | `docker compose config`の前に拒否する。 | P0 |
| E-09 | build context・dockerfile等が絶対pathまたはsource root外を参照する | HTTP 400、`PATH_OUTSIDE_PROJECT_ROOT`。入力された未解決pathだけを返す。 | P0 |
| E-10 | 実効Composeにbind mount、匿名volume、tmpfs、host port、privileged、device、Docker socket、host network/PID/IPCがある | 起動を拒否し、コンテナ、network、volumeを作成しない。 | P0 |
| E-11 | manifest指定serviceが存在しない、external network・volumeを含む | 推測・自動修正せず、登録または開始を失敗させる。 | P0 |
| E-12 | 未定義または名称不正の環境変数を保存する、必須値が不足する | 保存または起動を拒否する。secretをAPI・ログ・Operationに含めない。 | P0 |
| E-13 | 環境変数展開後の実効Composeが禁止構成になる | 起動・同期・再構成を拒否する。 | P0 |
| E-14 | SQLite migration失敗、DBロック、ディスク枯渇、整合性エラーが起きる | readyをfalseにし、状態変更を受け付けない。部分的な状態変更を残さない。 | P1 |
| E-15 | 未完了Operationのある同一appへ別の変更を要求する | HTTP 409。異なるappのOperationは上限2並列を維持する。 | P0 |
| E-16 | 同じ`requestId`を再送する、または同じIDで異なる内容を送る | 同一内容は同じOperationを返し、異なる内容は副作用なしで拒否する。 | P0 |
| E-17 | Operation実行中にBackendが再起動する | 未完了Operationを`failed`へ整理し、再起動後に正本から状態を再調整する。 | P1 |
| E-18 | Docker daemon停止、Docker API timeout、Compose CLI失敗、networkアドレス枯渇が起きる | Operationを`failed`にし、既存アプリを変更しない。secretを除去した短い日本語結果だけを残す。 | P0 |
| E-19 | 操作対象のproject名、owner label、installation ID、app-idのいずれかが不一致である | 停止、切断、削除を拒否する。LWS外Docker資源を操作しない。 | P0 |
| E-20 | Caddyfile検証・reload、hostsのatomic更新、CoreDNS再読込に失敗する | 既存Caddyfile・hosts・公開経路を維持してOperationを失敗にする。 | P0 |
| E-21 | 登録解除中にコンテナ停止後のsourceまたはnetwork削除に失敗する | 削除済み記録を消さず、再試行可能な状態を残す。 | P1 |
| E-22 | ACTIVEなアプリをpurgeする、確認済みでない、またはvolume所有情報が一致しない | 完全削除を拒否する。 | P0 |
| E-23 | 許可されないHost・Origin、または状態変更でJSON以外のContent-Typeを送る | Caddy経由以外の要求とみなし、CORSを許可せず拒否する。 | P0 |
| E-24 | SSEクライアントが切断、再接続、または低速である | workerを停止させず、無制限bufferを作らない。HTTP再取得で回復可能にする。 | P1 |

## 4. 自動テスト項目

### 4.1 Unitテスト

| ID | 対象 | 確認内容 | 対応シナリオ |
| --- | --- | --- |
| U-01 | base domain・公開IP設定 | 正常値、形式不正、値の欠損、既存設定を変更しない失敗を確認する。 | N-01, E-01〜03 |
| U-02 | 登録要求validator | repository URL、ref、subdomainの形式と重複を検証する。 | N-02, E-04 |
| U-03 | manifest validator | schema、サイズ、UTF-8、単一document、YAML Node制約、表示文字数を検証する。 | N-02, E-06〜07 |
| U-04 | Compose事前検査 | symlink、禁止キー、禁止外部参照、root外pathを検出する。 | E-08〜09 |
| U-05 | 実効Compose検査 | mount、network、port、privilege、device、external resource、公開service・portを検証する。 | E-10〜11, E-13 |
| U-06 | 環境変数 | 変数抽出、名称、必須性、default、secret暗号化、secret非表示を検証する。 | N-03, E-12〜13 |
| U-07 | DB・Operation | migration、foreign key、WAL、requestId冪等性、同一app直列化、再起動時の未完了Operation整理を検証する。 | N-06, E-14〜17 |
| U-08 | 派生設定生成 | hosts、Caddyfile、overrideがSQLiteの有効アプリ一覧だけから生成されることを検証する。 | N-02, N-04, N-09 |

### 4.2 HTTP契約テスト

| ID | 対象 | 確認内容 | 対応シナリオ |
| --- | --- | --- | --- |
| H-01 | OpenAPI | `backend/openapi.yaml`からの生成物に差分がないこと、全route・型が契約に従うことを確認する。 | 全体 |
| H-02 | 登録・更新・削除API | 成功時のJSON、Operation即時返却、resource name、etag、状態遷移を確認する。 | N-02, N-05〜08 |
| H-03 | エラー | AIP-193の`INVALID_ARGUMENT`、`BadRequest`、`ErrorInfo`、日本語messageを確認する。 | E-04, E-09〜13 |
| H-04 | HTTP境界 | Host、Origin、Content-Typeの許可・拒否とCORS非許可を確認する。 | E-23 |
| H-05 | secret保護 | configuration取得、エラー、Operation、ログAPIでsecret値が返らないことを確認する。 | N-03, E-12 |
| H-06 | SSE | event envelope、切断後の再接続、低速購読者の隔離を確認する。 | N-10, E-24 |

### 4.3 Docker境界テスト

Docker Engine API、Compose CLI、Git CLIをfakeに置き換え、引数、実行順、timeout、所有確認を検証する。

| ID | 確認内容 | 対応シナリオ |
| --- | --- | --- |
| D-01 | GitとComposeをshell文字列ではなくargvで実行し、timeoutを設定する。 | N-02, E-05, E-18 |
| D-02 | `compose.yaml`と生成overrideだけを`-f`指定し、`.env`と`compose.override.yaml`を暗黙に使わない。 | N-02, E-08, E-12 |
| D-03 | appごとのedge network、alias、Caddy接続、アプリ間接続拒否を確認する。 | N-04 |
| D-04 | 操作前にCompose project label、owner label、installation ID、app-idを検証する。 | E-19 |
| D-05 | 登録解除ではsource、runtime、volume、DB記録を残し、purgeでは所有確認済みresourceだけを削除する。 | N-07〜08, N-11, E-21〜22 |
| D-06 | `docker compose down --volumes`、`docker system prune`、`docker volume prune`を通常操作で呼ばない。 | N-06〜08, E-19 |
| D-07 | Docker失敗・network枯渇時に既存稼働構成への変更を行わない。 | E-18 |

### 4.4 統合テスト

| ID | 確認内容 | 対応シナリオ |
| --- | --- | --- |
| I-01 | 初回`lwsctl start`から、Backend、Caddy、CoreDNSが起動し、`api.<base-domain>`を名前解決できる。 | N-01 |
| I-02 | 最小の適合アプリを登録、設定、開始し、Caddy経由でHTTP応答を得る。 | N-02〜03 |
| I-03 | 同名serviceを持つ二つのアプリを登録し、各URLが正しいアプリへ到達し、相互通信できない。 | N-04 |
| I-04 | sync成功時に公開内容が切り替わり、sync失敗時に旧source・旧公開設定を維持する。 | N-05, E-05, E-20 |
| I-05 | 登録解除とpurgeで、コンテナ、network、source、volume、削除済み記録の差異を確認する。 | N-07〜08, E-21〜22 |
| I-06 | Backend再起動後にCaddy edge network接続と派生物を再調整する。 | N-09, E-17 |
| I-07 | CaddyfileまたはCoreDNS反映を意図的に失敗させ、既存の名前解決と公開経路が維持されることを確認する。 | E-20 |

## 5. 実装開始時のP0完了条件

InfrastructureとBackendの初回リリース候補では、少なくとも次を満たす。

1. `N-01`から`N-05`、`N-07`、`N-08`が自動テストで成功する。
2. `E-04`から`E-13`、`E-15`、`E-16`、`E-18`から`E-20`、`E-22`、`E-23`が自動テストで拒否される。
3. 正常・異常を問わず、LWS外Docker資源を操作しないことをfake Docker境界テストで確認する。
4. source更新、Caddyfile更新、hosts更新のいずれかが失敗しても、直前まで有効だったsourceと公開設定を維持する。
5. OpenAPI文書、生成物、HTTP契約テストに差分がない。
6. 関連する`mise`のlint、test、buildが成功する。

## 6. 実装順序

テスト可能な小さい単位で、次の順に実装する。

1. Backend基盤（設定、SQLite migration、OpenAPI生成、health endpoint）と`U-01`、`U-07`、`H-01`。
2. 共通validatorと`U-02`から`U-05`、`H-03`。
3. Operation worker、HTTP境界、環境変数管理と`U-06`、`H-02`、`H-04`、`H-05`。
4. Git取得、Docker所有確認、override・edge network、登録・開始と`D-01`から`D-04`、`I-02`から`I-03`。
5. hosts・Caddyfile生成、安全な反映、解除・purgeと`U-08`、`D-05`から`D-07`、`I-04`から`I-07`。
6. SSE、Backend再起動時再調整、残りのP1テスト。
