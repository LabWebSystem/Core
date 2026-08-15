# LabWebSystem

LabWebSystem（LWS）は、Docker Composeを基盤とするLAN向けWebアプリケーションプラットフォームです。現在の基盤では、Backend、Dashboard、SDKは意図的にモックコンポーネントとして実装しています。

## 開発

```sh
mise run lint
mise run test
mise run package
mise run deploy --version 1.2.3
mise run deploy --version 1.2.3 --force
```

`deploy`には認証済みの`gh` CLIが必要です。`lws-vX.Y.Z`タグをプッシュし、既存リリースを置き換える場合は、`--force`を指定しない限り確認します。

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
