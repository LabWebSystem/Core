# AGENTS.md

## 目的

このリポジトリは **LabWebSystem（LWS）** を管理します。

LWSは、研究室・家庭内LANなどの限定されたネットワーク内で、自作Webアプリを登録・公開・管理するためのシステムです。

明示的な設計変更がない限り、すべての変更は本ファイルで定義するアーキテクチャと責務分担を維持してください。

## 基本アーキテクチャ

責務分担は次のとおりです。

| 対象 | 責務 |
|---|---|
| 開発用ランタイム・ツール・タスク実行 | `mise` |
| Linuxへのインストール・削除・ファイル所有 | APT / DNF |
| LWS固有のライフサイクル操作 | `lwsctl` |
| 実行環境のオーケストレーション | Docker Compose |
| LWSパッケージ配布 | GitHub Releases |
| Dockerイメージ配布 | GHCR |
| TypeScript SDK配布 | GitHub Packages |
| CI/CD | GitHub Actions |

明示的な設計変更がない限り、これらの責務を別の層へ移さないでください。

禁止例:

- `lwsctl` がパッケージ管理下のファイルを独自に上書きする
- `.deb` / `.rpm` にDockerイメージを同梱する
- パッケージインストール時にLWSを自動起動する
- SDKがアプリ実行基盤を担当する
- Docker Compose以外をLWSの実行基盤の正本とする

## 開発原則

すべての実装で以下を必須原則とします。

### DRI — Don't Repeat Implementation

同じ処理、ルール、設定、コマンド列を複数箇所へ重複実装しないでください。

共通化可能なものは一つの実装へ集約し、利用側から参照してください。

例:

- CLIとBackendで同じ検証処理を別々に実装しない
- CI内にビルドコマンドを複製せず、原則として`mise`タスクを呼び出す
- DNSとReverse Proxyの設定を別々の状態として管理せず、共通の正本から生成する

### YAGNI — You Aren't Gonna Need It

現在の要件に必要なものだけを実装してください。

以下のような先回り実装は禁止します。

- 将来利用するかもしれない抽象化
- 未使用の拡張ポイント
- 不要なCLIコマンド
- 現時点で要件のない設定項目
- 現在利用しないインフラ構成

要件を満たす最小かつ一貫した変更を優先してください。

### SSoT — Single Source of Truth

重要な状態や設定には、必ず一つの正本を定めてください。

派生データは正本から生成し、独立して編集可能な複製を持たないでください。

例:

- 開発用コマンドの正本は`mise`タスク
- DNS / Reverse Proxy設定はLWSの永続状態から生成する
- インストール済みLWSバージョンと対応するDockerイメージのバージョンを一致させる
- アプリ情報は一つの権威ある状態表現を持つ

## リポジトリ構成

LWSはモノレポで管理します。

概念上、以下の要素を含みます。

```text
CLI
Dashboard
Backend
SDK
Infrastructure / Compose
Packaging
CI/CD
Tests
```

既存のディレクトリ構成がある場合は、それを優先してください。

不要なファイル移動や大規模リファクタリングは避けてください。

## `mise`

`mise` を開発時の正規エントリーポイントとします。

主なタスク例:

```text
mise run dev
mise run lint
mise run test
mise run build
mise run package
```

開発用ランタイムやツールは、可能な限り`mise`で管理してください。

CIでも可能な限り同じ`mise`タスクを利用してください。

## CLI契約

CLI名は `lwsctl` とします。

許可するコマンドは次の6個だけです。

```text
lwsctl start
lwsctl stop
lwsctl status
lwsctl rebuild
lwsctl update
lwsctl uninstall
```

仕様変更が明示されない限り、7個目のコマンドを追加しないでください。

### `start`

- 初回起動時に設定を作成する
- `--domain` / `-d` を受け付ける
- 設定済みドメイン変更時は確認する
- `--force` / `-f` で確認を省略できる
- DNS / Reverse Proxy設定を再調整する
- Docker Composeで起動する

### `stop`

LWS管理下のランタイムを安全かつ冪等に停止します。

### `status`

状態を変更せず、LWSの実行状態を表示します。

### `rebuild`

LWSのランタイムや生成設定を再構成します。

パッケージ再インストール機能にはしないでください。

### `update`

- 対応するLWSリリースを取得する
- パッケージ更新をAPT / DNFへ委譲する
- 対応するGHCRイメージを取得する
- 必要な再構成・再起動を行う

### `uninstall`

通常アンインストールでは、ランタイムとパッケージを削除し、永続データは保持します。

```text
lwsctl uninstall --purge
```

では、LWS設定・状態・永続データ・LWS管理下Docker volumeも削除します。

破壊的操作では、`--force` / `-f` がない限り確認を必須とします。

## ベースドメインとアプリURL

アプリは次の形式で公開します。

```text
<app-name>.<base-domain>
```

例:

```text
dashboard.example.internal
app-a.example.internal
```

ベースドメイン変更時は、関連するDNSとReverse Proxy設定を一貫して更新してください。

中途半端な状態を残してはいけません。

## Dockerと実行時安全性

LWSおよび登録アプリの実行基盤はDocker Composeです。

LWS管理下のDockerリソースは、必ずLWS所有であることを識別できるようにしてください。

無関係なコンテナ、ネットワーク、volumeを削除してはいけません。

通常のLWS操作で以下のような広範囲削除は禁止します。

```text
docker system prune
docker volume prune
```

削除対象はCompose project名、label等でLWS所有を明示的に確認してください。

## パッケージング

対象OS:

- Ubuntu系: `.deb` / APT
- AlmaLinux系: `.rpm` / DNF

パッケージには主に以下を含めます。

- `lwsctl`
- Compose定義
- 設定テンプレート
- lifecycle用スクリプト
- メタデータ

パッケージインストールはファイル配置のみとし、コンテナを起動しないでください。

APT / DNFで直接削除された場合でも、削除前hookからLWSを安全かつ冪等に停止してください。

パッケージ管理下のファイルはAPT / DNFの責務とします。

## コンポーネント責務

### Backend

以下を担当します。

- アプリ登録・削除
- Docker操作
- アプリ定義検証
- DNS設定
- Reverse Proxy設定
- LWS状態の永続化

外部リポジトリ由来の定義やユーザー入力は未信頼入力として扱ってください。

### Dashboard

管理UIを提供します。

Docker、DNS、ホスト設定を直接操作せず、Backend APIを利用してください。

### SDK

TypeScript SDKは、外部LWS対応アプリ向けの型、schema、検証、helperを提供します。

SDKはLWS本体とは独立してバージョン管理します。

外部アプリの必須ランタイム依存にはしないでください。

## リリース規約

LWS本体のタグ:

```text
lws-v<x.y.z>
```

Workflow:

```text
.github/workflows/release-lws.yml
```

同一LWSリリース内では、以下のバージョンを揃えてください。

```text
LWS package
lwsctl
compose
Backend image
Dashboard image
その他のLWS本体用image
```

SDKのタグ:

```text
sdk-v<x.y.z>
```

Workflow:

```text
.github/workflows/release-sdk.yml
```

SDKのバージョンはLWS本体から独立させます。

通常のpush / Pull Request用CIは以下とします。

```text
.github/workflows/ci.yml
```

通常CIからリリースしてはいけません。

## セキュリティ

以下は未信頼入力として扱ってください。

- GitHub Repository URL
- アプリ定義
- アプリ名
- ベースドメイン
- APIリクエスト

最低限、以下を守ってください。

- schema・hostnameを検証する
- path traversalを防止する
- 未信頼値をshell文字列へ直接展開しない
- 可能な限りargv形式でプロセス実行する
- root権限の利用範囲を最小化する
- secretをログへ出力しない
- LWS所有を確認できないDockerリソースを削除しない

LAN内利用であっても、入力検証や最小権限を省略してはいけません。

## テスト

変更内容に応じて適切なレベルのテストを追加・更新してください。

最低限、以下の契約を維持してください。

- CLI動作
- domain / hostname検証
- アプリ定義検証
- Backend動作
- Docker再調整
- DNS / Reverse Proxy生成
- install / update / remove lifecycle
- 通常uninstallとpurgeの差異
- LWS外のDockerリソースを削除しないこと

パッケージ処理はDebian系とRPM系の両方を考慮してください。

## 変更時のルール

実装時は次の順序を守ってください。

1. 既存実装を確認する
2. その処理の責務を持つコンポーネントを特定する
3. SSoTを維持する
4. 既存ロジックを再利用し、重複実装しない
5. 現在必要な範囲だけを実装する
6. 最小かつ一貫した変更を優先する
7. 関連テストを追加・更新する
8. 対応する`mise`タスクを実行する
9. 無関係なリファクタリングを避ける
10. 明示的な仕様変更がない限り、CLI・Packaging・永続化・Release契約を維持する

## 日本語化ルール

ユーザー向けの文章は日本語を正本とします。対象には、ドキュメント、README、CLIの標準出力・標準エラー、インストール・デプロイ用スクリプトのメッセージ、タスク説明、Workflowの表示名、パッケージ説明、MemADRの記述を含めます。

コマンド名、環境変数名、ファイル名、パス、URL、Gitタグ、Dockerイメージ名、YAML/TOML/JSONの予約キー、プログラム識別子、外部ツールが要求する固定値は契約の一部であるため翻訳しません。コード中のコメントも、技術的な識別子を除いて日本語で記述します。

新しいユーザー向け文言を追加するときは日本語で記述し、既存文言を変更するときは同じ領域に英語の説明を残さないでください。標準規格やライセンス本文など、原文の保持が必要な法的・外部契約文書は例外とします。

関連するlint、test、buildが成功して初めて変更完了とみなします。

## MemADR運用ポリシー

このリポジトリでは、MemADRを開発判断メモリとして使用する。

作業するエージェントは、必ず `MEMADR_WORKFLOW.md` を読み、その内容に従うこと。

特に、次を守ること。

- `memory/` に作業ログを書かない
- 現在も価値がある情報と、過去情報を区別する
- 古い判断、削除済み機能、無効化済み情報を現在有効な前提として扱わない
- MemADRレコードを追加または更新した場合は、完了前に `memadr check` と `memadr index` を実行する
