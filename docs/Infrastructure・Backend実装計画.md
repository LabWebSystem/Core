# Infrastructure・Backend 実装状況

本書は、完了済みの実装計画を現在の検証入口と実装状況へ置き換えた記録です。新しい仕様は、[Infrastructure仕様書](Infrastructure仕様書.md)、[Backend仕様書](Backend仕様書.md)、[機能定義書v1](relese/v1/機能定義書v1.md)を参照してください。

実装の正本はコードとテストです。利用者視点の受け入れテストの正本は、[機能テストルール](機能テストルール.md)と`qa/suites/`です。

## 現在の実装範囲

- CLI、パッケージ、release、Backend、OpenAPI、Docker境界、Infrastructureのテストtargetを実装済みです。
- Backendは、入力検証、SQLite正本、Operation、source切替、secret保護、Docker所有確認、DNS・Reverse Proxy反映を実装済みです。
- Operation状態SSE、コンテナログ収集、低速subscriberの制限、Backend再起動時の未完了Operation整理を実装済みです。
- Dashboardの基本的なアプリ管理画面と、APIモックを使うFrontend/E2Eテストを実装済みです。
- 実環境を必要とするv1受け入れ項目の多くは、引き続き`planned`または`isolated`です。

同一app-idのOperationは永続FIFOで処理し、異なるapp-idは最大2件まで並列実行します。同じ`requestId`の再送は既存Operationへ収束します。

## 検証入口

| コマンド | 対象 |
| --- | --- |
| `mise run test` | 全Core対象 |
| `mise run test cli` / `installer` / `release` | CLI、パッケージ、release |
| `mise run test backend` / `backend-http` / `backend-docker` | Backend、HTTP、Docker境界 |
| `mise run test infrastructure` | 実Compose、DNS、Caddy、network、volume |
| `mise run verify` / `verify full` | GitHub Actions品質ゲートの起動と結果取得 |

`mise run test`は全targetをローカルで実行します。通常CIはGitHub Actionsでfast、手動またはリリース前の全品質ゲートはGitHub Actionsでfullを実行します。`mise run verify`はpush済みコミットのActions実行を起動・監視します。CIのWorkflowは共通品質Workflowを呼び出し、各ジョブでは既存の`mise` taskを実行します。

## 未完了の受け入れ項目

内部テストと、利用者視点の実環境受け入れテストは別管理です。内部テストの成功だけで受け入れ項目を完了扱いにしません。未実装または専用環境が必要な項目は、`qa/suites/v1/`の`planned`または`isolated`タグで管理します。

受け入れテストを完了へ移すときは、対象のタグを更新し、関連する`mise`品質ゲートで実行できることを確認します。
