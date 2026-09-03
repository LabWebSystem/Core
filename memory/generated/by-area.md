<!-- This file is generated. Do not edit manually. -->

# By Area

## -
- ADR-001 | ACCEPTED | - | LWS配布とライフサイクルの基盤
- ADR-002 | ACCEPTED | - | InfrastructureとBackendの正本およびアプリ実行方式
- ADR-003 | ACCEPTED | - | Backend APIのAIP準拠とOpenAPI契約
- ADR-004 | ACCEPTED | - | Backendの応答性、SSE、およびOperation実行
- ADR-005 | ACCEPTED | - | アプリ単位のedge networkとCompose事前検査
- ADR-006 | ACCEPTED | - | Backend実装技術スタック
- ADR-007 | ACCEPTED | - | purge、原子性、起動時Docker再調整の方針
- ADR-008 | ACCEPTED | - | Infrastructure・Backend開発フェーズの現状
- ADR-009 | ACCEPTED | - | 機能受け入れテストをRobot Frameworkで分離する
- ADR-010 | ACCEPTED | - | miseを品質ゲートの正本とする
- ADR-011 | ACCEPTED | - | Dashboardはデバッグルームから段階的に洗練する
- ADR-012 | ACCEPTED | - | DashboardをReact・Chakra UIの同一画面管理クライアントとして実装する
- ADR-013 | ACCEPTED | - | Backend集約の永続ログ収集・検索・配信
- ADR-014 | ACCEPTED | - | 設定レイヤーとLWSデバイスプール
- CHG-001 | SHIPPED | - | Ubuntu系とAlmaLinux系のインストーラー分岐
- CHG-002 | SHIPPED | - | インストーラーのLWSリリース変数をOS情報から分離
- CHG-003 | SHIPPED | - | 初回起動時にベースドメインをComposeへ渡す
- CHG-004 | SHIPPED | - | lwsctlライフサイクルのモックテストを拡張
- CHG-005 | SHIPPED | - | LWS本体イメージをGHCRへ公開
- CHG-006 | SHIPPED | - | LWSバージョンの正本をパッケージ管理下へ移行
- CHG-007 | SHIPPED | - | lwsctlをGoバイナリとして配布する
- CHG-008 | SHIPPED | - | lwsctlのGo実装を責務ごとに分割する
- CHG-009 | SUPERSEDED | - | デプロイからSDKをGitHub Packagesへ公開する
- CHG-010 | SHIPPED | - | バージョン設定と公開操作を分離する
- CHG-011 | SHIPPED | - | miseの表示形式に依存しないリリーステスト
- CHG-012 | SHIPPED | - | GitHub ActionsのCIとリリースを段階並列化する
- CHG-013 | SHIPPED | - | RPM成果物名とインストーラーの探索条件を整合させる
- CHG-014 | SHIPPED | - | lwsctlの実行環境撤去をdownへ改名
- CHG-015 | SHIPPED | - | downで保存済み設定をComposeへ渡す
- CHG-016 | SHIPPED | - | updateで停止状態を維持する
- CHG-017 | SHIPPED | - | 未設定時のstatusを案内表示で正常終了する
- CHG-018 | SHIPPED | - | バージョン確認テストを非破壊化する
- CHG-019 | SHIPPED | - | Backend基盤とテストtargetを追加する
- CHG-020 | SHIPPED | - | Backend操作APIとworker境界を追加する
- CHG-021 | SHIPPED | - | Composeの必須環境変数展開をBackend検証へ移す
- CHG-022 | SHIPPED | - | Backend workerからCompose実行器へ接続する
- CHG-023 | SHIPPED | - | Docker所有確認と共有派生設定領域を追加する
- CHG-024 | SHIPPED | - | テストtargetの空実行を検出する
- CHG-025 | SHIPPED | - | 未信頼sourceと実効Composeの検証境界を強化する
- CHG-026 | SHIPPED | - | 設定値のsecret保護とHTTPエラー契約を強化する
- CHG-027 | SHIPPED | - | OpenAPI生成serverをHTTP入口へ接続する
- CHG-028 | SHIPPED | - | フェーズ2の入力・実効Compose検証を接続する
- CHG-029 | SHIPPED | - | フェーズ1・2の契約とruntime検証を接続する
- CHG-030 | SHIPPED | - | フェーズ3のOperation・source・Docker境界を実装する
- CHG-031 | SHIPPED | - | フェーズ3のDocker境界テストを補強する
- CHG-032 | SHIPPED | - | フェーズ3の更新Operationと失敗時復元を完成させる
- CHG-033 | SHIPPED | - | フェーズ3の未達受け入れ条件をテストで固定する
- CHG-034 | SHIPPED | - | フェーズ4未達項目のテスト先行実装
- CHG-035 | SHIPPED | - | フェーズ5のSSEと再起動耐障害性を実装する
- CHG-036 | SHIPPED | - | フェーズ0〜5の未達項目をテスト先行で補完する
- CHG-037 | SHIPPED | - | Backendの未検証境界を自動テストで補完する
- CHG-038 | SHIPPED | - | リリース時のBackend・Dashboard digestをパッケージへ反映する
- CHG-039 | SHIPPED | - | CIのCLIテストで実ホストport競合を分離する
- CHG-040 | SHIPPED | - | GitHub ActionsをNode.js 24対応版へ更新する
- CHG-041 | SHIPPED | - | CIのOSRunner timeoutテストを決定的にする
- CHG-042 | SHIPPED | - | LWSリリースWorkflowのBackend build contextとActionを修正する
- CHG-043 | SHIPPED | - | LWSリリースのDocker BuildKitキャッシュを有効化する
- CHG-044 | SHIPPED | - | 旧設定ファイルの権限をパッケージ更新時に移行する
- CHG-045 | SHIPPED | - | Robot Framework受け入れテスト基盤を追加する
- CHG-046 | SHIPPED | - | v1機能テスト項目をRobot Frameworkへ移植する
- CHG-047 | SHIPPED | - | RobotのAPI受け入れテストを実操作へ接続する
- CHG-048 | SHIPPED | - | LWS本体の起動ライフサイクルをRobotで検証する
- CHG-049 | SHIPPED | - | インストール受け入れテストをroot隔離実行へ変更する
- CHG-050 | SHIPPED | - | 機能テストの環境依存失敗をskipへ分類する
- CHG-051 | SHIPPED | - | API受け入れテストをCaddy公開経路へ接続する
- CHG-052 | SUPERSEDED | - | Robot FTプレースホルダーを実行可能な検証へ置換する
- CHG-053 | SHIPPED | - | 品質ゲートをmiseへ統合する
- CHG-054 | SHIPPED | - | 構造検査をgrepだけで実行する
- CHG-055 | SHIPPED | - | Docker inspectの未存在判定を修正する
- CHG-056 | SHIPPED | - | 品質ゲートを高速・QA・リリースへ分離し結果を集約する
- CHG-057 | SHIPPED | - | 機能テストルールを責務と実行プロファイルに合わせて再構成する
- CHG-058 | SHIPPED | - | miseの内部taskをライブラリ設定へ分離する
- CHG-059 | SHIPPED | - | READMEを現行の利用・開発入口に合わせて再構成する
- CHG-060 | SHIPPED | - | 初回起動前のLWS更新を許可する
- CHG-061 | SHIPPED | - | テスト用Dockerイメージを実行後に削除する
- CHG-062 | SHIPPED | - | Impeccableデザインスキルをプロジェクトへ導入する
- CHG-063 | SHIPPED | - | LWSのプロダクト文脈を定義する
- CHG-064 | SHIPPED | - | Dashboardデバッグルームと公開経路を実装する
- CHG-065 | SHIPPED | - | Dashboardのローカル結合確認環境を追加する
- CHG-066 | SHIPPED | - | Dashboardの公開URLとOperation進行表示を修正する
- CHG-067 | SHIPPED | - | Backend永続ログ収集・検索・SSE配信を実装する
- CHG-068 | SHIPPED | - | Operation待機列とSQLite書込み待機を追加する
- CHG-069 | SHIPPED | - | Backend正本のOperation進捗表示を追加する
- CHG-070 | SHIPPED | - | Dashboardのアプリ運用ログ面を構造化する
- CHG-071 | SHIPPED | - | 複数行JSONログをDashboardで構造化表示する
- CHG-072 | SHIPPED | - | 登録失敗の原因をBackend正本から表示する
- CHG-073 | SHIPPED | - | 登録解除OperationをWorkerへ投入する
- CHG-074 | SHIPPED | - | ログのseverity判定と条件付き表示を整理する
- CHG-075 | SHIPPED | - | 登録解除済みアプリをDashboardに表示する
- CHG-076 | SHIPPED | - | 完全削除の進捗とOperationを保持する
- CHG-077 | SHIPPED | - | アプリ資源と環境変数の管理情報をDashboardへ追加する
- CHG-078 | SHIPPED | - | Compose検証JSONを一行でOperationログへ記録する
- CHG-079 | SHIPPED | - | 設定レイヤーとリソースプールを追加する
- SUP-001 | ACTIVE | - | Robotの内部テスト代理実行を通常参照から外す

## backend
- BUG-013 | VERIFIED | backend | 未作成edge networkをDockerエラーとして扱う

## backend-operation
- BUG-016 | VERIFIED | backend-operation | 登録解除Operationが開始されない
- BUG-018 | OPEN | backend-operation | purge完了後にOperation履歴を取得できない

## cli
- BUG-002 | VERIFIED | cli | 初回startでComposeへベースドメインを渡さない
- BUG-003 | VERIFIED | cli | stopが保存済みのベースドメインをComposeへ渡さない
- BUG-004 | VERIFIED | cli | 設定済みのLWSバージョンをComposeへ渡さない
- BUG-007 | VERIFIED | cli | downが保存済みのベースドメインをComposeへ渡さない
- BUG-008 | VERIFIED | cli | updateが停止中のLWSを起動する
- BUG-009 | VERIFIED | cli | 未設定時のstatusがCompose補間エラーを返す
- BUG-014 | VERIFIED | cli | 初回起動前にupdateを実行できない

## infrastructure
- BUG-011 | VERIFIED | infrastructure | CoreDNSが生成hostsを読み取れない

## installer
- BUG-001 | VERIFIED | installer | AlmaLinuxでOSバージョンをLWSリリース版として参照する
- BUG-006 | VERIFIED | installer | アーキテクチャ名なしのRPMをインストーラーが見つけられない

## logging
- BUG-017 | VERIFIED | logging | 正常なstderrと複数行ログがerror表示される

## quality-gate
- BUG-015 | VERIFIED | quality-gate | テストで取得したDockerイメージが残る

## release
- BUG-005 | VERIFIED | release | checkoutなしのReleaseジョブでリポジトリを解決できない

## test
- BUG-010 | VERIFIED | test | テストがCoreバージョンを変更したまま終了する
- BUG-012 | VERIFIED | test | 品質ゲート構造検査が未管理のrgへ依存する
