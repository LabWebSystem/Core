# 変更履歴

## v0.1.10 - 2026-09-03

### 【機能追加】

- React・Vite・TypeScript・Chakra UI・TanStack Queryによる日本語Dashboardを追加し、アプリ登録、起動・停止、同期、再構成、設定、ログ、登録解除、完全削除を管理画面から操作できるようにした。
- Dashboardを`dashboard.<base-domain>`で公開し、Caddy経由でBackend APIへ接続する構成を追加した。
- Operationのフェーズ、表示メッセージ、SSE通知をDashboardへ表示し、登録や同期の進捗を確認できるようにした。
- Docker・Operation・LWS基盤のログをBackendへ永続保存し、検索、カーソルによる再開、SSE配信、secretマスク、保持期限・容量に基づく削除を追加した。
- 同一アプリのOperationを永続FIFO待機列で処理し、SQLiteの短時間の書き込み競合を待機するようにした。
- DashboardでアプリのOperationログと複数サービスの運用ログを構造化表示し、サービス・期間・件数による絞り込みに対応した。
- 登録解除済みアプリを一覧に表示し、再登録と完全削除をDashboardから実行できるようにした。
- 完全削除の進捗と完了Operationを保持し、削除中の操作を制限するようにした。
- プロジェクトへImpeccableのデザイン支援手順とLWSのプロダクト文脈を追加した。

### 【バグ修正】

- 登録解除Operationが待機状態のまま開始されない問題を修正した。
- 正常なstderrや複数行JSONログがerror表示・分割表示になる問題を修正した。
- Gitのref不在や重複公開名など、登録失敗の原因がDashboardに表示されない問題を修正した。

### 【仕様変更】

- DashboardのAPI契約をOpenAPIから生成した型と同一Originの公開経路に統一した。
- Operationとログの状態をBackend・SQLiteを正本として扱い、SSEは再取得可能な通知経路とした。
- 完全削除は関連するDocker資源と保存データの削除完了後にアプリ記録を削除する順序へ変更した。

## v0.1.9 - 2026-08-23

### 【機能追加】

- QA、Dashboard検証、実Docker統合テストで、実行前後に増えたテスト用Dockerイメージだけを自動削除する仕組みを追加した。

### 【仕様変更】

- `lwsctl update`で固定digestのイメージがすでにローカルにある場合は、不要な`compose pull`を行わないようにした。

## v0.1.8 - 2026-08-23

### 【機能追加】

- Robot Frameworkによる利用者視点の受け入れテスト基盤を追加し、CLI、API、ライフサイクル、アプリ操作、インストールを検証できるようにした。
- `mise run verify`を品質ゲートの正本とし、fast、qa、releaseの検証プロファイルと、結果を集約したMarkdownレポートを追加した。
- BackendのDocker resource検査で、未作成のedge networkを検出して作成できるようにした。
- 初回`start`前でも、設定ファイルやコンテナを作成せずに`lwsctl update`でパッケージとDockerイメージを更新できるようにした。

### 【バグ修正】

- 品質ゲートが管理対象外の`rg`に依存して実行できない問題を修正した。
- 初回アプリ登録時、未作成のedge networkをDockerエラーとして扱い登録に失敗する問題を修正した。
- 初回起動前の`lwsctl update`が設定未作成を理由に失敗する問題を修正した。

### 【仕様変更】

- 機能テストを内部テストと分離し、Robot Frameworkを利用者ワークフローの正本とした。
- 通常の`verify`、QAを含む`verify qa`、パッケージ生成まで含む`verify release`に検証範囲を分離した。
- `mise`の内部taskをライブラリ設定へ分離し、利用者向けtaskをルート設定の主要入口として整理した。
- READMEと機能テスト規則を現行のCLI、Backend、QA、品質ゲートの構成に合わせて更新した。

## v0.1.7 - 2026-08-17

### 【バグ修正】

- non-rootで動作するCoreDNSとCaddyがBackend生成のhosts・Caddyfileを読み取れず、管理ドメインが名前解決できない問題を修正した。

## v0.1.6 - 2026-08-17

### 【バグ修正】

- パッケージ更新後、旧版が作成した`/etc/lws/config.env`の権限を新しい`lwsctl`が拒否して再実行できない問題を修正した。

### 【仕様変更】

- Debianの`preinst`とRPMの`%pre`で既存設定の権限を`0600`へ移行するようにした。

## v0.1.5 - 2026-08-17

### 【機能追加】

- LWSリリースWorkflowでDocker BuildKitのGitHub Actionsキャッシュを利用し、Backend・Dashboardイメージのビルドを高速化した。

### 【仕様変更】

- Buildxのイメージdigestを後続のパッケージ生成へ直接引き渡し、リリース時のイメージ参照を固定する構成にした。

## v0.1.4 - 2026-08-17

### 【機能追加】

- Go製Backendを追加し、SQLiteによる状態永続化、OpenAPI 3.1 API、アプリ登録・更新・設定、非同期Operation、SSE、Git取得、Docker Compose実行を実装した。
- アプリ定義とComposeの検証、manifest検証、secretの暗号化保存、LWS所有Docker resourceの確認を追加した。
- BackendがSQLiteを正本としてCoreDNS hosts、Caddyfile、アプリ用Compose overrideを生成し、DNSとReverse Proxyを再調整できるようにした。
- アプリごとのedge network、名前付きvolume、source/runtime領域を管理し、登録・同期・起動・停止・登録解除・purgeを実行できるようにした。
- 同一アプリのOperation直列化、異なるアプリの最大2並列、requestId冪等性、失敗時のsource・公開経路復元を実装した。
- Backend起動時のDocker再調整、旧`config.env`からの設定移行、`lwsctl down --purge`によるLWS所有resource限定削除を追加した。
- OperationとコンテナログのSSE、切断後の再接続、Backend再起動時の未完了Operation整理を追加した。
- CI、LWSリリース、SDKリリースを分離・段階並列化し、Backend・Dashboardイメージとパッケージの公開処理を整備した。

### 【バグ修正】

- BackendのDockerfileが参照するリポジトリルートのファイルを、リリースWorkflowの誤ったbuild contextから修正した。
- CIのCLIテストで、GitHub Actions runnerの53番ポート使用を本番のポート競合と誤判定する問題を修正した。
- OSRunnerのtimeoutテストが子プロセスの標準出力待ちで不安定になる問題を修正した。
- RPM成果物名とAlmaLinuxインストーラーの探索条件を修正し、x86_64環境でインストールできるようにした。

### 【仕様変更】

- LWS本体、SDK、Dockerイメージのバージョン管理と公開操作を分離し、各コンポーネントを独立してリリースできるようにした。
- OpenAPIをBackend APIの正本とし、生成server・型・clientを利用する契約へ変更した。
- 外部Composeのbind mount、host namespace、host port、外部network・volume、symlinkなどを拒否し、許可するnamed volumeとLWS所有resourceを明確化した。
- アプリ登録解除はsource、runtime、設定、volume、DB記録を保持する`UNREGISTERED`への遷移とし、完全削除と分離した。
- Docker操作はargv形式で実行し、Composeの実効構成を`up`前に検証するようにした。

## v0.1.3 - 2026-08-16

### 【バグ修正】

- 停止中のLWSへ`lwsctl update`を実行しても、更新後に意図せず起動しないように修正した。
- 初期設定前の`lwsctl status`がCompose補間エラーを表示する問題を修正した。
- バージョン確認テストが`version/core`を固定値へ書き換えたまま終了する問題を修正した。

### 【仕様変更】

- `lwsctl update`は更新前の稼働状態を引き継ぎ、停止中なら停止状態を維持するようにした。
- 未設定時の`lwsctl status`はComposeを実行せず、設定状態と`start`の案内を表示して正常終了するようにした。

## v0.1.2 - 2026-08-16

### 【バグ修正】

- `lwsctl down`が保存済みのベースドメインとLWSバージョンをComposeへ渡さず、設定済み環境の撤去に失敗する問題を修正した。

## v0.1.1 - 2026-08-16

### 【仕様変更】

- `lwsctl uninstall`を`lwsctl down`へ変更し、実行環境の撤去とAPT・DNFによるパッケージ削除の責務を分離した。
- 通常の`down`は設定・状態・永続データを保持し、`down --purge`だけがLWS管理下のデータとDocker volumeを削除するようにした。

## v0.1.0 - 2026-08-16

### 【機能追加】

- LWSの初期配布基盤として、Ubuntu系の`.deb`、AlmaLinux系の`.rpm`、GitHub Releases、GHCRイメージ公開を追加した。
- `lwsctl`をGoバイナリとしてパッケージに含め、起動、停止、状態確認、再構成、更新、実行環境撤去のライフサイクル操作とヘルプを提供した。
- `/usr/share/lws/version`をLWSバージョンの正本とし、Composeが対応するBackend・Dashboardイメージを参照できるようにした。
- `mise`によるビルド・テスト・パッケージ・リリース入口と、LWS本体・SDKの独立したGitHub Actions Workflowを追加した。

### 【バグ修正】

- Ubuntu系とAlmaLinux系で適切なパッケージマネージャーと成果物を選択できるようにした。
- AlmaLinuxの`VERSION`がLWSリリース指定を上書きするインストーラーの問題を修正した。
- 初回`start`で保存したベースドメインがComposeへ渡らない問題を修正した。
- LWS本体イメージの参照先を実在するGHCR名へ修正した。
- リリースWorkflowがcheckoutなしでGitHub Releaseを作成できない問題を修正した。
- x86_64 RPMの成果物名とインストーラーの探索条件を整合させた。

### 【仕様変更】

- LWSのライフサイクル操作を`lwsctl`へ集約し、パッケージインストール時にはコンテナを自動起動しない構成とした。
- LWS設定・永続状態をホスト上の所定領域で管理し、DNS・Reverse Proxy設定を状態から生成する方針を定めた。
- LWSリリースとSDKリリースを独立したタグとWorkflowで管理するようにした。
