# LabWebSystem Backend仕様書

## 1. 目的と責務

BackendはLWSのアプリ管理と実行状態の唯一の管理者である。HTTP APIを提供し、未信頼入力を検証し、SQLiteへ正本を保存し、その正本からDNS・Reverse Proxy設定とLWS用Compose overrideを生成する。

Backendはアプリの登録、設定値の管理、開始、停止、同期、削除、Gitリポジトリとmanifestの検証、LWS所有Dockerリソースの操作を担当する。パッケージ操作、LWS本体Composeの開始停止、パッケージ管理下ファイルの変更、DNSプロトコルの実装、任意Composeの書換えは担当しない。

## 2. 正本と状態

SQLiteを`/var/lib/lws/database.sqlite`に置き、WAL modeとforeign key制約を有効にする。

| エンティティ | 主な内容 |
| --- | --- |
| `applications` | app-id、subdomain、リポジトリURL、ref、manifestの表示情報、望ましい状態、取得済みrevision、最終エラー、登録状態、保持volumeの識別情報、時刻 |
| `application_variables` | app-id、変数名、secretフラグ、暗号化済み値または通常値、更新時刻 |
| `operations` | operation ID、app-id、種別、状態、時刻、エラー概要 |

app-idはBackendが発行するUUIDである。subdomainは`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`に一致し、同一base domain内で一意でなければならない。公開URLは`<subdomain>.<base-domain>`である。

`compose.yaml`と`lws.manifest.yaml`はアプリsourceの正本である。以下は派生物であり、手編集しない。

```text
/var/lib/lws/apps/<app-id>/runtime/lws.override.yaml
/var/lib/lws/apps/<app-id>/runtime/app.env
/var/lib/lws/generated/Caddyfile
/var/lib/lws/generated/hosts
```

コンテナ、network、volumeの実在と状態はDockerから都度照会し、DBへ複製しない。

登録状態は`ACTIVE`または`UNREGISTERED`である。登録解除後も、完全削除までapp-id、installation ID、保持volumeの識別情報だけを削除済み記録として保持する。`UNREGISTERED`なApplicationは通常の一覧から除外し、開始、停止、同期、再構成を受け付けない。

## 3. API設計規則と契約

Backend APIはGoogle API Improvement Proposals（AIP）に従う。特に、resource-oriented design（AIP-121）、標準メソッド（AIP-131からAIP-135）、custom method（AIP-136）、HTTP statusとerror modelの規約を適用する。

- APIはリソースを中心に設計し、DB tableをそのまま公開しない。
- 標準操作はList、Get、Create、Update、Deleteを優先する。
- 標準操作で表せない実行操作だけを、`POST /api/v1/{name=applications/*}:start`のようなresourceベースcustom methodとして表す。
- custom methodはHTTP動詞を増やさず、URI末尾の`:verb`で表す。
- resource nameは`applications/{application}`および`operations/{operation}`とし、URL parameterとレスポンスの`name`へ同じ値を使う。
- JSON fieldはlowerCamelCase、列挙値はUPPER_SNAKE_CASEとする。secretはoutput-only fieldを含め、どのAPI responseにも返さない。
- エラーはAIP-193に従う。入力不正はHTTP 400と`INVALID_ARGUMENT`で返し、`google.rpc.ErrorInfo`のUPPER_SNAKE_CASEな`reason`を機械可読な識別子とする。field単位の不正は`google.rpc.BadRequest`の`fieldViolations`へ含める。

API契約の唯一の正本は`backend/openapi.yaml`のOpenAPI 3.1文書とする。HTTP route、request/response型、入力検証、TypeScript API clientはこの文書から生成する。業務ロジックは生成済みserver interfaceを実装し、同じresource型、path、schema、validation ruleを手書きで重複定義してはならない。

OpenAPI文書と生成物の差分がある場合はCIを失敗させる。契約変更は、OpenAPI文書、生成物、互換性テストを同じ変更で更新する。`mise`に定義する生成taskだけが生成物を更新する。

Protobufは、将来gRPCまたは双方向streamingが実要件になった場合に再評価する。現時点では、gRPC transport、HTTP transcoding、protobufとOpenAPIの二重契約を導入しない。

## 4. 通信プロトコルと応答性

通常のAPIはHTTP/1.1 keep-alive上のREST/JSONを使用する。HTTPが、すべてのresource取得、設定変更、Operation作成、および障害復旧の正本である。

SSEはサーバーから管理クライアントへの一方向通知だけに使用する。初期実装ではOperationの状態変更とコンテナログの追跡をSSEで配信する。SSE接続は状態の正本ではないため、切断またはイベント欠損時はHTTPでApplicationまたはOperationを再取得する。

WebSocketは将来の高頻度メトリクス差分などに追加できるよう、SSEのイベントを`eventId`、`sequence`、`timestamp`、`type`、`data`からなる共通envelopeで表す。初期実装ではWebSocket endpoint、接続管理、client APIを実装しない。WebSocketを追加する場合も、操作要求とresource取得をWebSocketへ移さず、HTTP APIを正本のまま維持する。

Backendは次の応答性規則を守る。

- 変更要求はSQLiteへcommitしてOperationを作成した直後に返し、clone、build、Docker操作の完了を待たない。
- Applicationは`desiredState`、`observedState`、`reconciling`、`latestOperation`、`etag`を返す。望ましい状態とDockerの観測状態を混同しない。
- ListおよびGetのDocker状態は、所有ラベルで一括取得した短期in-memory snapshotから返し、`observedAt`を含める。操作直後は即時にsnapshotを更新する。
- 変更要求はUUID v4の`requestId`を受け付ける。同じ操作と`requestId`の再送は、同じOperationを返し副作用を重複させない。
- 管理クライアントによる再試行は、`requestId`がある変更要求とread requestだけに限定する。再試行は指数backoffとjitterを使用する。

### 4.1 SLIとSLO

SLOはGoogle SREのSLI、SLO、error budgetの考え方に従い、28日間のrolling windowで測定する。単一ホスト構成であるため、可用性はホスト稼働中のBackend APIを対象とする。

| SLI | SLO |
| --- | --- |
| Read API latency | 99%が200ms以下、99.9%が500ms以下 |
| 変更要求の受領とOperation作成 | 99%が200ms以下、99.9%が500ms以下 |
| SSEによる状態変更通知 | 95%が1秒以下 |
| ログイベントの到達 | 95%が500ms以下、99%が2秒以下 |
| Backend API availability | 99.9% |
| 同一`requestId`の重複副作用 | 0件 |

UIのend-to-end指標は、75パーセンタイルのINPを200ms以下とする。Operation完了時間はアプリのclone、build、起動時間に依存するためSLOにしない。代わりに、受領の速さ、状態通知、最終結果の永続化を測定する。

## 5. HTTP API

API prefixは`/api/v1`とする。成功・失敗レスポンスはJSONで返し、ユーザー向けメッセージは日本語にする。

初期実装は認証・認可を持たない。到達可能なLAN内端末を管理者として扱い、HTTP通信上のsecretの秘密性を保証しない。BackendはCaddy経由以外の到達を受け付けず、CORSを許可しない。状態変更要求はJSONの`Content-Type`と許可済み`Host`を満たさなければ拒否する。`Origin` headerがあるブラウザ要求は許可済みoriginだけを受け付ける。

| メソッド | パス | 処理 |
| --- | --- | --- |
| `GET` | `/health/live` | プロセスの稼働状態 |
| `GET` | `/health/ready` | DB、永続ディレクトリ、Docker接続の利用可否 |
| `GET` | `/applications` | `ListApplications`。アプリ一覧と実行状態 |
| `POST` | `/applications` | `CreateApplication`。リポジトリを登録し、Operationを返す |
| `GET` | `/applications/{application}` | `GetApplication`。アプリ定義、manifest情報、設定状況、実行状態 |
| `PATCH` | `/applications/{application}` | `UpdateApplication`。`updateMask`で変更可能fieldを指定 |
| `DELETE` | `/applications/{application}` | `UnregisterApplication`。登録解除Operationを返す |
| `POST` | `/applications/{application}:purge` | `PurgeApplication`。完全削除Operationを返す |
| `GET` | `/applications/{application}/configuration` | `GetApplicationConfiguration`。必要な変数の定義と設定状況。値は返さない |
| `PATCH` | `/applications/{application}/configuration` | `UpdateApplicationConfiguration`。変数値を更新 |
| `POST` | `/applications/{application}:start` | `StartApplication`。Operationを返す |
| `POST` | `/applications/{application}:stop` | `StopApplication`。Operationを返す |
| `POST` | `/applications/{application}:sync` | `SyncApplication`。Operationを返す |
| `POST` | `/applications/{application}:rebuild` | `RebuildApplication`。Operationを返す |
| `GET` | `/operations/{operation}` | `GetOperation`。Operationの状態と結果 |
| `GET` | `/operations/{operation}:watch` | `WatchOperation`。`text/event-stream`でOperation状態を配信 |
| `GET` | `/applications/{application}:tailLogs` | `TailApplicationLogs`。`text/event-stream`でコンテナログを配信 |

登録要求は`repositoryUrl`、`ref`、`subdomain`を含む。表示名と説明はmanifestから読む。長時間かかるCreate、登録解除、完全削除、custom methodは`Operation` resourceを返す。未完了Operationがあるapp-idへの新規変更は409で拒否する。

登録解除はコンテナ、アプリ用edge network、source、runtimeを削除し、named volumeと削除済み記録を保持する。完全削除は明示的な確認済み要求だけが実行できる。保持volumeをproject名、LWS所有ラベル、installation ID、app-idで確認して削除し、すべてのvolume削除に成功した後だけ削除済み記録を消す。volume削除に失敗した場合は削除済み記録を保持してOperationを失敗にする。

`PATH_OUTSIDE_PROJECT_ROOT`のような検証理由は、人間向けメッセージとは別に`ErrorInfo.reason`で返す。たとえばsource root外の`services.app.build.context`はHTTP 400、`INVALID_ARGUMENT`、`PATH_OUTSIDE_PROJECT_ROOT`、該当fieldの`fieldViolations`を返す。bind mountはプロジェクト内のパスであっても禁止であるため、`BIND_MOUNT_FORBIDDEN`を返す。エラーには利用者が指定した未解決パスだけを含め、ホスト上の解決済み絶対パスやsecretを含めない。

```json
{
  "error": {
    "code": 400,
    "message": "指定されたパス \"../../foo/bar\" は許可された範囲外です。プロジェクトディレクトリ配下を参照する場合は \"./foo/bar\" のように指定してください。",
    "status": "INVALID_ARGUMENT",
    "details": [
      {
        "@type": "type.googleapis.com/google.rpc.BadRequest",
        "fieldViolations": [
          {
            "field": "services.app.build.context",
            "description": "指定されたパス \"../../foo/bar\" はプロジェクトディレクトリの外を参照しています。"
          }
        ]
      },
      {
        "@type": "type.googleapis.com/google.rpc.ErrorInfo",
        "reason": "PATH_OUTSIDE_PROJECT_ROOT",
        "domain": "labwebsystem"
      }
    ]
  }
}
```

## 6. リポジトリ、manifest、Composeの検証

受け付けるリポジトリURLは`https://github.com/<owner>/<repository>`または末尾`.git`付きだけとする。認証情報、query、fragment、`ssh://`、`git@`、`file://`、ローカルパス、GitHub以外のhostを拒否する。Git操作は対話入力を無効化し、argv形式で実行する。

cloneは一時ディレクトリで行い、`compose.yaml`と`lws.manifest.yaml`の検証成功後だけ`source/`へ原子的に入れ替える。失敗時は既存sourceと実行設定を保持する。

manifestの正規ファイル名、サイズ、YAML構文、完全schemaは[Infrastructure仕様書](Infrastructure仕様書.md)に従う。Backendは共通schemaを唯一の実装として使用し、HTTP handlerごとに検証を重複実装しない。

manifestの`public.service`は実効Compose内に存在しなければならない。Backendはサービス名やportを推測しない。`metadata.name`と`metadata.description`は、検証済みsourceの更新時に更新する。

Backendは`source/compose.yaml`と生成した`runtime/lws.override.yaml`だけを`docker compose -f`で明示指定する。リポジトリの`compose.override.yaml`、`.env`、追加Composeファイルを暗黙に使わない。

`docker compose config`を呼ぶ前に、Backendはsource treeにsymlinkがないこと、外部ファイル読込機能がないこと、許可するpath参照がsource root配下へ解決されることを検証する。この段階で拒否するCompose機能は、`include`、`extends`、`env_file`、`label_file`、`volumes_from`、ファイル型`configs`または`secrets`、`build.additional_contexts`、リモートGit build contextである。pathを丸めたり、Composeを変更したりして受理してはならない。事前検査後に`docker compose config --format json`で実効モデルを検証し、標準Composeの意味を独自に再実装しない。

## 7. 環境変数

Composeの変数参照はアプリ構成の正本である。BackendはCompose CLIの変数出力を使い、必要な変数名、必須性、Compose側のdefault有無を抽出する。値は推測しない。

値の入力は管理クライアントが受け付けるが、検証、保存、secret保護、`app.env`の生成はBackendの責務である。管理クライアントはリポジトリ内の`.env`を作成・更新せず、保存済み値を読取らない。

- Backendはsourceの`.env`を使用しない。空の明示的`--env-file`で変数を分析し、実行時は生成済み`runtime/app.env`だけを指定する。
- 変数名は`^[A-Z_][A-Z0-9_]*$`に限定する。未定義の変数値は保存できない。
- 必須変数が未設定なら、起動・同期・再構成を失敗させる。defaultを持つ変数は値を保存しなくてもよい。
- 値を反映した実効Composeを必ず再検証する。値によって禁止構成が現れた場合は起動しない。
- secretフラグがある値は`/etc/lws/secret.key`で暗号化してDBへ保存する。API、ログ、Operation結果、状態表示に値を含めない。
- `app.env`はBackendが起動直前に0600で生成し、LWSだけが読める権限を維持する。

Composeでコンテナへ渡る環境変数は通常の環境変数機構を使用する。secretフラグはLWSの保存・表示・ログの扱いを定めるものであり、アプリが環境変数として受け取る事実を変えない。

## 8. OperationとDocker操作

時間のかかる操作はHTTPリクエスト中に完了させず、Operationとして実行する。状態は`queued`、`running`、`succeeded`、`failed`、`cancelled`とする。Backend起動時の未完了Operationは`failed`へ整理する。

同一app-idのOperationは一つだけ実行する。すでに同種のOperationが`queued`または`running`である場合は、そのOperationを返す。未開始の古い操作と矛盾する新しい望ましい状態の操作は`cancelled`へ遷移させる。異なるapp-idのOperationは、resource消費量を制限するbounded worker poolで並列に実行する。初期値の並列数は2とする。

| 種別 | 処理 |
| --- | --- |
| `start` | 検証済み実効Composeでアプリを起動し、DNSとProxyを同期 |
| `stop` | 所有確認済みアプリComposeを停止し、DNSとProxyを同期 |
| `sync` | sourceを取得してmanifest、変数、実効Composeを再検証し、再起動 |
| `rebuild` | sourceを取得せず、overrideとapp.envを作り直して再起動 |
| `unregister` | アプリを停止し、Caddyをapp-idのedge networkから切断してから、source、runtime、所有確認済みedge networkを削除。named volumeと削除済み記録を保持 |
| `purge` | `UNREGISTERED`なアプリの所有確認済みnamed volumeを削除し、全volume削除成功後に削除済み記録を消去 |

app-idのCompose project名は`lws-app-<app-id>`で固定する。BackendはDocker操作の前に、Docker Composeが自動付与する`com.docker.compose.project`がこのproject名と一致することを確認する。LWS生成overrideが付与したcontainer labelでは、さらに`com.labwebsystem.owner=lws`、installation ID、role=`application`、app-idの一致を確認する。volumeの削除ではprojectラベルを必須とし、LWS本体または他のapp-idのprojectを対象にしない。

Docker呼出しにはtimeoutを設定し、出力からsecretを除去して短い日本語のOperation結果へ記録する。未検証値をshell文字列、Docker引数、path、label、override YAMLへ直接展開してはならない。通常の停止・同期・再構成で`docker compose down --volumes`を使わず、`docker system prune`や`docker volume prune`を使わない。

## 9. 同期、ファイル操作、テスト

公開状態を変えるOperationは、Compose成功後にDBから有効アプリ一覧を読み、Caddyfileとhostsを生成、Caddyfileを検証、atomic rename、所有確認済みCaddyの無停止再読込の順で実行する。失敗時は既存生成物を保持してOperationを失敗にする。

Backendが書くホストファイルは`/var/lib/lws`だけである。app-idを検証し、real pathが対象root外へ出ないことを確認する。DBにないディレクトリ、または所有確認できないDockerリソースを自動削除しない。

少なくとも次を自動テストする。

- リポジトリURL、subdomain、manifest完全schema、変数名、path traversal、symlinkの拒否
- manifest指定サービスの存在確認と、公開サービスを推測しないこと
- `compose.override.yaml`とsourceの`.env`をLWS実行へ取り込まないこと
- bind mount、匿名volume、host port、privileged、Docker socket、host network/PID/IPC、device、external resourceの拒否
- 登録解除でnamed volumeと削除済み記録を保持し、完全削除で所有確認済みvolumeを削除した後だけ記録を消すこと
- 変数未設定時の起動拒否、secret非表示、変数展開後の実効Compose再検証
- `include`、`extends`、`env_file`、`label_file`、`volumes_from`、ファイル型`configs`/`secrets`、外部path、remote build contextを`docker compose config`実行前に拒否すること
- source root外のpathに`PATH_OUTSIDE_PROJECT_ROOT`とfield violationを返し、bind mountに`BIND_MOUNT_FORBIDDEN`を返すこと
- 同じCompose service名を持つ複数アプリを、一意な`lws-<app-id>` aliasで正しいupstreamへ転送すること
- アプリ間の直接通信を拒否し、Caddyだけが各アプリ用edge networkへ接続すること
- Caddy経由以外のBackend API到達、許可されないOrigin、JSONでない状態変更要求を拒否すること
- `requestId`の再送で同一Operationを返し、重複したDocker操作を実行しないこと
- 同一app-idのOperation直列化、異なるapp-idの上限付き並列実行、矛盾する未開始Operationのcancel
- SSEのevent ID、再接続、HTTPによる状態復旧、遅いログ購読者の上限付きbuffer
- LWS外Dockerリソースを操作しないこと
- Caddyfile検証失敗と同期失敗時に既存設定を保持すること
