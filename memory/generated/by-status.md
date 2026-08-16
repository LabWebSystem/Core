<!-- This file is generated. Do not edit manually. -->

# By Status

## ACCEPTED
- ADR-001 | ACCEPTED | - | LWS配布とライフサイクルの基盤
- ADR-002 | ACCEPTED | - | InfrastructureとBackendの正本およびアプリ実行方式
- ADR-003 | ACCEPTED | - | Backend APIのAIP準拠とOpenAPI契約
- ADR-004 | ACCEPTED | - | Backendの応答性、SSE、およびOperation実行
- ADR-005 | ACCEPTED | - | アプリ単位のedge networkとCompose事前検査
- ADR-006 | ACCEPTED | - | Backend実装技術スタック

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

## SUPERSEDED
- CHG-009 | SUPERSEDED | - | デプロイからSDKをGitHub Packagesへ公開する

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
