# LabWebSystem Backend仕様書

## 1. 責務と実装技術

Backendは、アプリ管理と実行状態の唯一の管理者である。HTTP APIを提供し、未信頼入力を検証し、SQLiteの正本からDNS、Reverse Proxy、LWS用Compose overrideを生成する。

| 担当する | 担当しない |
| --- | --- |
| アプリ登録、設定値、開始、停止、同期、登録解除、完全削除 | パッケージ操作、LWS本体Composeの開始・停止、パッケージ管理下ファイルの変更 |
| Gitリポジトリ・manifest・Composeの検証 | DNSプロトコルの実装、任意Composeの書換え |
| LWS所有Dockerリソースの操作 |  |

| 領域 | 採用技術・方針 |
| --- | --- |
| HTTP | Go 1.26と標準`net/http`。Web frameworkは導入しない |
| API | OpenAPI 3.1を正本とし、固定versionの`oapi-codegen`で`std-http-server`、strict server interface、Go型を生成する。`kin-openapi` middlewareでrequestを検証する |
| 管理API client | Dashboardなどの管理クライアントの責務。BackendフェーズではOpenAPI契約を提供し、clientの言語・生成方式は後続フェーズで決める |
| DB | SQLite、`database/sql`、CGO不要の`modernc.org/sqlite`。ORMを使わずSQLを明示する |
| migration | SQL migrationを`go:embed`で同梱し、起動時に一度だけ適用する |
| Docker | Docker CLIをargv形式・timeout付きで実行する。network・container・volume操作と`docker compose`をCLIへ委譲する |
| YAML | `gopkg.in/yaml.v3`のNode ASTでmanifestとComposeを事前検査する |
| 非同期・ログ | SQLiteのOperationとBackend内worker pool。`log/slog`でJSON構造化ログを出し、secretを記録しない |
| テスト | 標準`testing`、`httptest`、fake Docker/CLI。HTTP、DB、検証、Docker境界を分けて検証する |

- 依存ライブラリと生成器は`go.mod`と生成設定でversionを固定する。`oapi-codegen`はOpenAPI 3.1対応のv2.8.0以降を使い、更新時は生成物と互換性テストを更新する。
- Redis、メッセージキュー、外部job基盤、Kubernetes、Compose意味の独自実装は導入しない。

## 2. 正本と状態

- SQLiteは`/var/lib/lws/database.sqlite`に置き、WAL modeとforeign key制約を有効にする。

| エンティティ | 主な内容 |
| --- | --- |
| `applications` | app-id、subdomain、repository URL、ref、manifest表示情報・公開service・公開port、desired state、revision、最終エラー、登録状態、時刻 |
| `application_variables` | app-id、変数名、secretフラグ、暗号化済み値または通常値、更新時刻 |
| `operations` | operation ID、app-id、種別、状態、時刻、エラー概要 |

- app-idはBackend発行UUID、subdomainは`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`かつbase domain内で一意とする。公開URLは`<subdomain>.<base-domain>`である。
- `compose.yaml`と`lws.manifest.yaml`がアプリsourceの正本である。`runtime/lws.override.yaml`、`runtime/app.env`、生成Caddyfile、生成hostsは派生物である。Backend起動時と状態変更後に派生物を再調整する。
- コンテナ、network、volumeの実在と状態はDockerから都度照会し、DBへ複製しない。
- 登録状態は`ACTIVE`と`UNREGISTERED`。`UNREGISTERED`はcontainerとedge networkを持たないが、アプリ設定、source、runtime、named volume、DB記録を保持する復帰可能状態である。通常の一覧から除外し、開始・停止・同期・再構成を受け付けない。Docker resourceの実体と所有識別情報はlabelから照会し、DBへ複製しない。

## 3. API契約

- Google AIP（AIP-121、AIP-131〜135、AIP-136、AIP-193）に従う。resource-oriented design、標準メソッド、custom method、HTTP statusとerror modelを適用する。
- resource nameは`applications/{application}`と`operations/{operation}`。JSON fieldはlowerCamelCase、列挙値はUPPER_SNAKE_CASEとする。
- API契約の唯一の正本は`backend/openapi.yaml`のOpenAPI 3.1文書である。HTTP route、型、schema validationはここから生成・検証する。管理clientの生成はDashboard以降の責務とし、業務ロジックは生成済みserver interfaceへ委譲する。
- OpenAPI文書と生成物が不一致ならCIを失敗させる。生成物の更新は`mise`の生成taskだけが行う。
- Protobuf、gRPC、HTTP transcodingは、双方向streamingの実要件が出るまで導入しない。

### 3.1 HTTP API

- prefixは`/api/v1`。成功・失敗レスポンスはJSON、利用者向けメッセージは日本語とする。
- 認証・認可は持たない。到達可能なLAN端末を管理者として扱う。BackendはCaddy経由以外を受け付けず、CORSを許可しない。`dashboard.<base-domain>`から同一Originの`/api/v1`へ送られた要求だけを許可し、任意の`Origin`は拒否する。
- 状態変更はJSONの`Content-Type`と許可済み`Host`を要求する。`Origin` headerがある要求は許可済みoriginだけを受け付ける。

| メソッド | パス | 処理 |
| --- | --- | --- |
| `GET` | `/health/live`、`/health/ready` | 稼働状態、依存先の利用可否 |
| `GET` / `POST` | `/applications` | 一覧、登録（Operationを返す） |
| `GET` / `PATCH` / `DELETE` | `/applications/{application}` | 取得、更新、登録解除（Operationを返す） |
| `POST` | `/applications/{application}:purge` | 完全削除（Operationを返す） |
| `GET` / `PATCH` | `/applications/{application}/configuration` | 変数定義・設定状況、値の更新。secret値は返さない |
| `POST` | `/applications/{application}:register`、`:start`、`:stop`、`:sync`、`:rebuild` | アプリ操作（Operationを返す） |
| `GET` | `/operations/{operation}`、`/operations/{operation}:watch` | Operation取得、SSEによる状態配信 |
| `GET` | `/applications/{application}:tailLogs` | SSEによるコンテナログ配信 |

- 登録要求は`repositoryUrl`、`ref`、`subdomain`を含む。表示名と説明はmanifestから読む。
- 長時間操作はOperationを返す。未完了Operationがあるapp-idへの新規変更は409で拒否する。
- 登録解除はアプリcontainerとapp用edge networkだけを削除し、source、runtime、named volume、アプリ設定、UNREGISTEREDのDB記録を保持する。再登録は保持したsource・設定を使って復帰する。完全削除は`UNREGISTERED`かつ`confirm:true`の要求だけが実行でき、アプリデータ、source、runtime、所有確認済みvolumeを削除してからDB記録を物理削除する。途中失敗時は記録を保持する。

### 3.2 エラー

- AIP-193に従う。入力不正はHTTP 400と`INVALID_ARGUMENT`を返す。
- `google.rpc.ErrorInfo.reason`はUPPER_SNAKE_CASEの機械可読な理由、field単位の不正は`google.rpc.BadRequest.fieldViolations`とする。
- `PATH_OUTSIDE_PROJECT_ROOT`はsource root外のpath、`BIND_MOUNT_FORBIDDEN`はbind mountに使う。エラーには未解決の入力pathだけを含め、ホストの絶対pathやsecretを含めない。

```json
{"error":{"code":400,"message":"指定されたパス \"../../foo/bar\" は許可された範囲外です。プロジェクトディレクトリ配下を参照する場合は \"./foo/bar\" のように指定してください。","status":"INVALID_ARGUMENT","details":[{"@type":"type.googleapis.com/google.rpc.BadRequest","fieldViolations":[{"field":"services.app.build.context","description":"指定されたパス \"../../foo/bar\" はプロジェクトディレクトリの外を参照しています。"}]},{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"PATH_OUTSIDE_PROJECT_ROOT","domain":"labwebsystem"}]}}
```

## 4. 通信と応答性

- 通常APIはHTTP/1.1 keep-alive上のREST/JSONを使う。resource取得、設定変更、Operation作成、障害復旧の正本はHTTPである。
- SSEはOperation状態とコンテナログの一方向通知に使う。切断・欠損時はHTTPでApplicationまたはOperationを再取得する。
- WebSocketは初期実装しない。将来追加する場合もHTTPを正本とし、SSEと共通の`eventId`、`sequence`、`timestamp`、`type`、`data` envelopeを使う。

| 規則 | 内容 |
| --- | --- |
| 即時応答 | 変更要求はSQLiteへcommitしOperationを作成した直後に返す。clone、build、Docker完了を待たない |
| 状態 | Applicationは`desiredState`、`observedState`、`reconciling`、`latestOperation`、`etag`を返す |
| Docker照会 | 所有labelで一括取得した短期in-memory snapshotを使い、`observedAt`を返す。操作直後は更新する |
| 冪等性 | 変更要求はUUID v4の`requestId`を受け、同一要求の再送には同じOperationを返す |
| 再試行 | `requestId`付き変更とreadだけを、指数backoffとjitterで再試行する |
| 並列性 | 同一app-idは直列化し、異なるapp-idは上限2のworker poolで並列化する |

28日rolling windowのSLOは次とする。

| SLI | SLO |
| --- | --- |
| Read API・変更要求の受領とOperation作成 | 99%が200ms以下、99.9%が500ms以下 |
| SSE状態変更通知 | 95%が1秒以下 |
| ログイベント到達 | 95%が500ms以下、99%が2秒以下 |
| Backend API availability | 99.9%（ホスト稼働中を対象） |
| 同一`requestId`の重複副作用 | 0件 |

UIのINPは75パーセンタイル200ms以下とする。clone、build、起動に依存するOperation完了時間はSLOにしない。

## 5. リポジトリ、Compose、環境変数

### 5.1 取得と検証

- 受理するURLは`https://github.com/<owner>/<repository>`または末尾`.git`付きだけ。認証情報、query、fragment、SSH URL、`file://`、ローカルpath、GitHub以外のhostを拒否する。
- Git操作は対話入力を無効化し、argv形式で行う。cloneは一時ディレクトリへ行い、`compose.yaml`とmanifestの検証成功後だけ`source/`へ原子的に入れ替える。失敗時は既存sourceと実行設定を保つ。
- manifestのschemaは[Infrastructure仕様書](Infrastructure仕様書.md)を唯一の仕様とし、Backendは共通validatorを使う。
- `docker compose config`前に、source treeのsymlink、`include`、`extends`、`env_file`、`label_file`、`volumes_from`、ファイル型`configs`/`secrets`、`build.additional_contexts`、リモートGit build context、root外pathを拒否する。pathを丸めたりComposeを変更して受理しない。
- LWS実行では`source/compose.yaml`と生成overrideだけを`-f`指定する。`compose.override.yaml`、sourceの`.env`、追加Composeファイルを暗黙に使わない。事前検査後にだけ`docker compose config --format json`で実効モデルを検証する。

### 5.2 環境変数

- Composeの変数参照から、必要な変数名、必須性、default有無を抽出する。値は推測しない。
- 値の入力は管理クライアント、検証・保存・secret保護・`app.env`生成はBackendが担当する。sourceの`.env`を作成・更新・使用しない。
- 変数名は`^[A-Z_][A-Z0-9_]*$`。未定義変数の値は保存できない。必須値がなければ起動・同期・再構成を失敗させる。
- secretは`/etc/lws/secret.key`で暗号化してDBへ保存し、API、ログ、Operation結果、状態表示に含めない。secret keyはsymlinkを拒否し、既存ファイルは0600だけを受理する。`app.env`は起動直前に0600で生成する。
- 値を反映した実効Composeを再検証し、禁止構成が現れた場合は起動しない。

## 6. Docker操作とテスト

- Operation状態は`queued`、`running`、`succeeded`、`failed`、`cancelled`。Backend再起動時の未完了Operationは`failed`へ整理する。
- project名は`lws-app-<app-id>`で固定する。操作前にCompose project label、LWS所有label、installation ID、app-idを確認する。
- 登録解除はアプリを停止してからCaddyをedge networkから切断し、所有確認済みedge networkを削除した後にDBを`UNREGISTERED`へ確定する。source、runtime、named volume、アプリ設定は削除しない。DB確定前の失敗ではACTIVE状態と公開経路を補償復元する。完全削除は`UNREGISTERED`なアプリだけが対象で、APIは`confirm:true`を必須とする。
- Docker呼出しにはtimeoutを設定し、出力からsecretを除去して短い日本語のOperation結果へ残す。未検証値をshell文字列、Docker引数、path、label、override YAMLへ直接展開しない。
- `docker compose down --volumes`、`docker system prune`、`docker volume prune`を通常操作で使わない。

少なくとも次を自動テストする。

- URL、subdomain、manifest schema、変数名、path traversal、symlink、禁止Compose機能の拒否
- manifest指定serviceの存在、source `.env`・`compose.override.yaml`非取込み、変数展開後の再検証
- bind mount、匿名volume、host port、privileged、Docker socket、host network/PID/IPC、device、external resourceの拒否
- `PATH_OUTSIDE_PROJECT_ROOT`、`BIND_MOUNT_FORBIDDEN`、secret非表示、HTTP origin・content-type制約
- named volumeの登録解除時保持、完全削除時の所有確認と削除済み記録の消去
- 同名Compose serviceのalias分離、アプリ間通信拒否、Caddyだけのedge network接続
- `requestId`の冪等性、Operation直列化、上限付き並列、SSE再接続、LWS外Dockerリソース非操作
- Caddyfile検証・同期失敗時に既存設定を保持すること
