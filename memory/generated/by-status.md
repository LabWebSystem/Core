<!-- This file is generated. Do not edit manually. -->

# By Status

## ACCEPTED
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

## ACTIVE
- SUP-001 | ACTIVE | - | Robotの内部テスト代理実行を通常参照から外す

## SHIPPED
- CHG-001 | SHIPPED | - | Ubuntu系とAlmaLinux系のインストーラー分岐
- CHG-002 | SHIPPED | - | インストーラーのLWSリリース変数をOS情報から分離
- CHG-003 | SHIPPED | - | 初回起動時にベースドメインをComposeへ渡す
- CHG-004 | SHIPPED | - | lwsctlライフサイクルのモックテストを拡張
- CHG-005 | SHIPPED | - | LWS本体イメージをGHCRへ公開
- CHG-006 | SHIPPED | - | LWSバージョンの正本をパッケージ管理下へ移行
- CHG-007 | SHIPPED | - | lwsctlをGoバイナリとして配布する
- CHG-008 | SHIPPED | - | lwsctlのGo実装を責務ごとに分割する
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
- CHG-053 | SHIPPED | - | 品質ゲートをmiseへ統合する

## SUPERSEDED
- CHG-009 | SUPERSEDED | - | デプロイからSDKをGitHub Packagesへ公開する
- CHG-052 | SUPERSEDED | - | Robot FTプレースホルダーを実行可能な検証へ置換する

## VERIFIED
- BUG-001 | VERIFIED | installer | AlmaLinuxでOSバージョンをLWSリリース版として参照する
- BUG-002 | VERIFIED | cli | 初回startでComposeへベースドメインを渡さない
- BUG-003 | VERIFIED | cli | stopが保存済みのベースドメインをComposeへ渡さない
- BUG-004 | VERIFIED | cli | 設定済みのLWSバージョンをComposeへ渡さない
- BUG-005 | VERIFIED | release | checkoutなしのReleaseジョブでリポジトリを解決できない
- BUG-006 | VERIFIED | installer | アーキテクチャ名なしのRPMをインストーラーが見つけられない
- BUG-007 | VERIFIED | cli | downが保存済みのベースドメインをComposeへ渡さない
- BUG-008 | VERIFIED | cli | updateが停止中のLWSを起動する
- BUG-009 | VERIFIED | cli | 未設定時のstatusがCompose補間エラーを返す
- BUG-010 | VERIFIED | test | テストがCoreバージョンを変更したまま終了する
- BUG-011 | VERIFIED | infrastructure | CoreDNSが生成hostsを読み取れない
