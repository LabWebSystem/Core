<!-- This file is generated. Do not edit manually. -->

# By Status

## ACCEPTED
- ADR-001 | ACCEPTED | - | LWS配布とライフサイクルの基盤

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

## SUPERSEDED
- CHG-009 | SUPERSEDED | - | デプロイからSDKをGitHub Packagesへ公開する

## VERIFIED
- BUG-001 | VERIFIED | installer | AlmaLinuxでOSバージョンをLWSリリース版として参照する
- BUG-002 | VERIFIED | cli | 初回startでComposeへベースドメインを渡さない
- BUG-003 | VERIFIED | cli | stopが保存済みのベースドメインをComposeへ渡さない
- BUG-004 | VERIFIED | cli | 設定済みのLWSバージョンをComposeへ渡さない
- BUG-005 | VERIFIED | release | checkoutなしのReleaseジョブでリポジトリを解決できない
