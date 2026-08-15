# LabWebSystem

LabWebSystem（LWS）は、Docker Composeを基盤とするLAN向けWebアプリケーションプラットフォームです。現在の基盤では、Backend、Dashboard、SDKは意図的にモックコンポーネントとして実装しています。

## 開発

```sh
mise run lint
mise run test
mise run package
mise run version
mise run version core 1.2.3
mise run version sdk 0.4.0
mise run release core sdk
mise run release all
```

`version`は引数なしで各コンポーネントの現在バージョンを一覧表示し、対象とバージョンを指定するとその正本を更新します。`release`には認証済みの`gh` CLIが必要で、指定した`core`または`sdk`の現在のバージョンをタグとして公開し、対応するWorkflowの完了まで待機します。`all`は両方を公開します。既存Coreリリースを置き換える場合は、`--force`を指定します。公開済みSDKバージョンはGitHub Packagesの仕様上、再公開できません。

## インストールとライフサイクル

GitHub上のインストーラーは[LabWebSystem/Core](https://github.com/LabWebSystem/Core)の`scripts/install.sh`です。フォークを使う場合は`LWS_REPOSITORY=owner/repository`を設定してください。

```sh
curl -fsSL https://raw.githubusercontent.com/LabWebSystem/Core/main/scripts/install.sh | sudo bash
sudo lwsctl start --domain example.internal
sudo lwsctl stop
sudo lwsctl uninstall
sudo lwsctl uninstall --purge --force
```

パッケージインストールはファイル配置だけを行います。Composeプロジェクトを起動するのは`lwsctl start`です。パッケージマネージャーを直接使用した場合も、APT/DNFの削除前hookがプロジェクトを停止します。LWSは専用のComposeプロジェクトとラベルを使うため、無関係なDockerリソースは削除しません。

インストーラーは`/etc/os-release`を確認し、Ubuntu系ではGitHub Releasesから`.deb`を取得してAPTで、AlmaLinux系では`.rpm`を取得してDNFでインストールします。対応していないOSや必要なパッケージマネージャーがない環境では、コンテナやパッケージを変更せず日本語のエラーを表示して終了します。
