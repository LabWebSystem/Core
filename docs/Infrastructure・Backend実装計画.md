# Infrastructure・Backend 実装計画

## 1. 目的

本計画は、[Infrastructure仕様書](Infrastructure仕様書.md)、[Backend仕様書](Backend仕様書.md)、および[シナリオテスト計画](Infrastructure・Backendシナリオテスト計画.md)を満たすInfrastructureとBackendを、検証可能な小さな変更単位で実装するための順序を定める。

仕様と受け入れテストの正本はそれぞれ上記の文書である。本書は実装の依存関係、成果物、完了条件だけを定める。

## 2. 実施原則

- `mise run test`は全テストを実行する正規入口として維持する。
- 実装対象ごとのテストは`mise run test <target>`で実行可能にする。
- 各フェーズでは、受け入れテストを先に追加または更新し、それを通す最小の実装だけを加える。
- 未信頼なComposeをDockerへ渡す前の検証と、Docker資源の所有確認を先行させる。
- Docker API・Git・Compose CLIの境界はfakeで検証し、実Dockerを使う統合テストは代表シナリオだけに限定する。
- フェーズ完了時には、該当target、`mise run lint`、`mise run test`を実行する。

## 3. テストタスクの構成

### 3.1 利用者向けの実行入口

| コマンド | 対象 | 導入フェーズ |
| --- | --- | --- |
| `mise run test` | 全対象 | フェーズ0 |
| `mise run test cli` | `lwsctl`の単体・モック境界テスト | フェーズ0 |
| `mise run test installer` | Debian/RPMのinstall・remove lifecycleテスト | フェーズ0 |
| `mise run test release` | releaseスクリプトのモックテスト | フェーズ0 |
| `mise run test backend` | BackendのUnit、DB、validatorテスト | フェーズ1以降 |
| `mise run test backend-http` | OpenAPI生成・HTTP契約・handlerテスト | フェーズ1以降 |
| `mise run test backend-docker` | fake Docker、Compose、Git境界テスト | フェーズ3以降 |
| `mise run test infrastructure` | 実Compose、Caddy、CoreDNS、edge network、volumeの統合テスト | フェーズ4以降 |

`test`の引数なし実行は全targetを順に実行する。CIは対象別の並列jobへ分けてもよいが、通常CIとリリース前のどちらでも全targetの成功を必須とする。

### 3.2 分割時の制約

- 現在の`test_help`、`test_lifecycle`、`test_domain_change`、`test_update`、`test_update_running`、`test_down`は`cli`へ移す。
- AlmaLinux向けasset解決は`installer`、fake `git`・`gh`を使う公開導線は`release`へ移す。
- 各targetは独立した一時ディレクトリ・環境変数・fixtureを使い、実行順に依存しない。
- target間で同じsetup・fake・fixtureを複製せず、テストヘルパーに集約する。
- 実Docker統合テストは専用のCompose project名、専用の一時状態ディレクトリ、専用networkを使い、完了時に所有確認済み資源だけを削除する。

## 4. フェーズ

### フェーズ0: 既存テストの細分化

**目的:** 現在のCLI・パッケージ・releaseの回帰検出能力を維持したまま、以後のBackend／Infrastructureテストを追加できる入口を作る。

**変更:**

- `mise run test [target]`の引数を受け付けるようにする。
- 現在の`scripts/test.sh`を`cli`、`installer`、`release`の独立した対象へ分ける。
- 引数なしでは全対象を実行し、既存CIの`mise run test`を変更せずに通す。

**完了条件:**

- 既存のテストケースが、分割前と同じ観測内容を維持する。
- `mise run test`と各既存targetが成功する。
- targetを指定しない新規・削除テストは作らない。

### フェーズ1: Backend基盤と契約

**目的:** API、DB、起動判定を安全に追加するための正本とテスト基盤を作る。

**変更:**

- `backend/openapi.yaml`をAPI契約の正本として追加し、固定versionの生成器からserver interface、型、管理clientを生成する。
- Go Backendの起動処理、設定読込、SQLite接続、migration、`/health/live`、`/health/ready`を実装する。
- DBでWAL modeとforeign key制約を有効にし、起動時にmigrationを一度だけ適用する。
- `mise run test backend`と`mise run test backend-http`の土台を追加する。

**先行テスト:** `U-01`、`U-07`のmigration・起動部分、`H-01`、health endpoint。

**完了条件:**

- OpenAPI文書と生成物の不一致をCIが検出する。
- DB初期化失敗時にreadyを返さず、状態変更を受け付けない。
- health endpointとmigrationがテストで確認できる。

### フェーズ2: 未信頼入力の検証

**目的:** Git取得やDocker操作の前に、入力とComposeを安全に拒否する境界を完成させる。

**変更:**

- repository URL、subdomain、manifest、環境変数名の共通validatorを実装する。
- YAML Node ASTを使い、manifestの厳格なschema検証を実装する。
- source treeとComposeの事前検査、`docker compose config --format json`後の実効Compose検査を実装する。
- AIP-193形式のHTTPエラーと日本語の利用者向けmessageを実装する。

**先行テスト:** `U-02`〜`U-05`、`H-03`、`E-04`〜`E-13`。

**完了条件:**

- 禁止された入力はDocker・Gitを実行する前に拒否される。
- root外pathは`PATH_OUTSIDE_PROJECT_ROOT`、bind mountは`BIND_MOUNT_FORBIDDEN`を返す。
- エラーにsecretまたはホスト絶対pathを含めない。

### フェーズ3: Operation、アプリsource、Docker境界

**目的:** 登録・同期・開始を非同期かつ冪等に実行し、LWS所有のDocker資源だけを操作する。

**変更:**

- Application、application variable、Operationの永続化とworker poolを実装する。
- `requestId`冪等性、同一app直列化、異なるappの最大2並列を実装する。
- Git cloneとsourceの原子的切替、環境変数保存と`app.env`生成を実装する。
- appごとのoverride、edge network、network alias、Caddy接続を実装する。
- Docker／Compose／Git呼出しにargv形式、timeout、所有label・installation ID・app-id確認を実装する。

**先行テスト:** `U-06`、`U-07`のOperation部分、`H-02`、`H-04`、`H-05`、`D-01`〜`D-04`、`N-02`〜`N-05`、`E-15`〜`E-19`。

**完了条件:**

- 同一の`requestId`は同じOperationを返し、同じappの競合変更は409となる。
- source切替・Docker操作の失敗時に、既存sourceと既存アプリを維持する。
- fake Docker境界テストで、LWS外資源に対する操作が一度も発行されない。

### フェーズ4: Infrastructure反映と削除

**目的:** SQLiteの正本からDNS・Reverse Proxyを安全に反映し、登録解除と完全削除を所有範囲内で完結させる。

**変更:**

- LWS本体ComposeへBackend、Caddy、CoreDNS、internal network、必要な永続領域を追加する。
- SQLiteからhosts、Caddyfile、LWS用overrideを一時ファイルへ生成し、atomic renameで反映する。
- Caddyfile検証後の無停止reloadとCoreDNS hosts再読込を実装する。
- 登録解除、purge、保持volume識別情報、通常`down`と`down --purge`の差を実装する。
- `mise run test infrastructure`で実Dockerの代表シナリオを実行する。

`infrastructure` targetは、Caddy・CoreDNS・複数のapp edge networkを実Dockerで起動し、公開経路、名前解決、network分離、reload、反映失敗時の旧設定維持、volume所有範囲を検証する。

**先行テスト:** `U-08`、`D-05`〜`D-07`、`I-01`〜`I-07`、`N-07`〜`N-08`、`E-20`〜`E-22`。

**完了条件:**

- CaddyまたはCoreDNSの反映失敗時に、旧hosts・旧Caddyfile・旧公開経路を維持する。
- 登録解除ではnamed volumeを保持し、purgeでは所有確認済みvolumeだけを削除する。
- 実Compose統合テストで、DNS、Caddy経由の公開、app間通信拒否を確認する。

### フェーズ5: リアルタイム通知と耐障害性

**目的:** 管理操作の観測性と復旧性をP1要件まで完成させる。

**変更:**

- Operation状態とコンテナログのSSEを実装する。
- Backend再起動時に未完了Operationを`failed`へ整理し、SQLiteからCaddy接続と派生物を再調整する。
- DB障害、Docker timeout、低速SSE購読者、ポート競合のテストを追加する。

**先行テスト:** `H-06`、`N-09`〜`N-10`、`E-02`、`E-03`、`E-14`、`E-17`、`E-24`。

**完了条件:**

- SSEの切断・欠損はHTTP再取得で回復できる。
- Backend再起動後に、未完了OperationとLWS管理下のDocker構成が安全に再調整される。

## 5. CIへの反映

フェーズ0では既存の`mise run test`を維持する。BackendまたはInfrastructureのtargetが追加された時点で、通常CIは少なくとも次のjobへ分ける。

| job | 実行内容 |
| --- | --- |
| CLI・パッケージ | `mise run test cli`、`mise run test installer`、`mise run test release` |
| Backend | `mise run test backend`、`mise run test backend-http`、`mise run test backend-docker` |
| Infrastructure | `mise run test infrastructure` |
| 全体 | `mise run test` |

全体jobは、targetの配線漏れを検出するために残す。実Docker統合テストにDocker daemonまたは特権実行が必要な場合だけ、その環境を提供できるCI jobへ隔離する。環境がなければテストを黙ってskipせず、必要なCI実行基盤を先に整える。

## 6. 完了の判定

P0完了は、シナリオテスト計画の「実装開始時のP0完了条件」を全て満たし、通常CIで全テストtargetが成功することとする。P1はフェーズ5まで完了した時点で判定する。
