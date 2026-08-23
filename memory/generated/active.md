<!-- This file is generated. Do not edit manually. -->

# Active
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
- BUG-012 | VERIFIED | test | 品質ゲート構造検査が未管理のrgへ依存する
- BUG-013 | VERIFIED | backend | 未作成edge networkをDockerエラーとして扱う
- BUG-014 | VERIFIED | cli | 初回起動前にupdateを実行できない
- BUG-015 | VERIFIED | quality-gate | テストで取得したDockerイメージが残る
- SUP-001 | ACTIVE | - | Robotの内部テスト代理実行を通常参照から外す
