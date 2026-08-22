# LabWebSystem

LabWebSystem（LWS）は、Docker Composeを基盤とするLAN向けWebアプリケーションプラットフォームです。v0.1.7では、CLIとBackendを使ったアプリ登録・公開・管理の基盤を利用できます。利用者向けの操作方法は[LWS v0.1.7 利用マニュアル](docs/LWS%20v0.1.7利用マニュアル.md)を参照してください。

## 開発

```sh
mise run lint
mise run test
mise run test architecture
mise run verify         # fast（開発中の既定）
mise run verify qa      # fast + 統合テスト + 隔離Robot QA
mise run verify release # qa + パッケージ生成
mise run package
mise run version
mise run version core 1.2.3
mise run version sdk 0.4.0
mise run release core sdk
mise run release all
```

`verify`はプロファイルで範囲を選ぶ品質ゲートです。引数なしの`fast`は開発中に必要な静的検査、Core高速テスト、SDK、Dashboardを実行します。`qa`は実Docker統合テストと隔離Robot QAを追加し、`release`はさらにパッケージを生成します。各実行の統一要約と生ログは`test/result/YYYY-MM-DD-verify-result.md`へ保存されます。GitHub Actionsは`fast`、LWSリリースは`release`を実行します。

`version`は引数なしで各コンポーネントの現在バージョンを一覧表示し、対象とバージョンを指定するとその正本を更新します。`release`には認証済みの`gh` CLIが必要で、指定した`core`または`sdk`の現在のバージョンをタグとして公開し、対応するWorkflowの完了まで待機します。`all`はCoreとSDKを並列に公開し、両方のWorkflow完了まで待機します。出力には`[LWS]`または`[SDK]`が付き、関連するステップだけを表示します。既存Coreリリースを置き換える場合は、`--force`を指定します。公開済みSDKバージョンはGitHub Packagesの仕様上、再公開できません。

## インストールとライフサイクル

GitHub上のインストーラーは[LabWebSystem/Core](https://github.com/LabWebSystem/Core)の`scripts/install.sh`です。フォークを使う場合は`LWS_REPOSITORY=owner/repository`を設定してください。

```sh
curl -fsSL https://raw.githubusercontent.com/LabWebSystem/Core/main/scripts/install.sh | sudo bash
sudo lwsctl start --domain example.internal
sudo lwsctl stop
sudo lwsctl down
sudo lwsctl down --purge --force
```

パッケージインストールはファイル配置だけを行います。Composeプロジェクトを起動するのは`lwsctl start`です。`lwsctl down`は実行環境だけを削除し、パッケージを削除する場合はAPT/DNFを使用します。パッケージマネージャーを直接使用した場合も、削除前hookがプロジェクトを撤去します。LWSは専用のComposeプロジェクトとラベルを使うため、無関係なDockerリソースは削除しません。

インストーラーは`/etc/os-release`を確認し、Ubuntu系ではGitHub Releasesから`.deb`を取得してAPTで、AlmaLinux系では`.rpm`を取得してDNFでインストールします。対応していないOSや必要なパッケージマネージャーがない環境では、コンテナやパッケージを変更せず日本語のエラーを表示して終了します。
