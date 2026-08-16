# LabWebSystem（LWS）設計概要

## 🧭 システムの概要

LabWebSystem（LWS）は、研究室・家庭内LANなどの限定されたネットワーク内で、自作Webアプリを簡単に登録・公開・管理するためのシステムです。

* 対象OSはUbuntu系およびAlmaLinux系。
* Docker / Docker Composeを実行基盤として使用。
* DNS、Reverse Proxy、Dashboard、BackendなどのシステムコンポーネントはDocker Composeで管理。
* WebアプリはGitHubリポジトリ単位で管理し、Dashboardから登録。
* 登録されたWebアプリは、LWSで設定したベースドメイン配下のサブドメインとして公開。
* DNSとReverse Proxyを介して、LAN内から名前解決・アクセス可能にする。
* LWSに適合する外部Webアプリの開発を支援するため、TypeScript SDKを提供する。
* ソースコードは基本的にモノレポで管理。

  * CLI
  * Dashboard
  * Backend
  * SDK
  * Infrastructure / Compose
  * Packaging
  * CI/CD
* 開発時のツール・ランタイム管理およびタスクランナーとして `mise` を使用し、開発ホストへの依存関係の直接導入を最小化する。
* DockerイメージはGHCRで管理。
* `.deb` / `.rpm` はGitHub Releasesで配布。
* TypeScript SDKはGitHub Packagesで配布。
* OSへのインストール、ファイル配置、削除はAPT/DNFの既存エコシステムへ委譲。
* CLIツールは `lwsctl` とし、LWS固有のライフサイクル操作のみを担当する。

CLIは次の6コマンドに限定します。

```text
lwsctl start
lwsctl stop
lwsctl status
lwsctl rebuild
lwsctl update
lwsctl down
```

---

## 📖 用語集

* **LWS**：LabWebSystemの略称。本システム全体を指す。
* **ベースドメイン**：LWSでWebアプリを公開する際の基準となるドメイン。例：`example.internal`。
* **サブドメイン**：ベースドメイン配下で個々のWebアプリを識別するドメイン。例：`app-a.example.internal`。
* **Infrastructure**：LWSの実行基盤に関する構成。Docker Compose、DNS、Reverse Proxy、ネットワークなどを含む。
* **Dashboard**：WebブラウザからLWSを管理するためのWeb UI。Webアプリの登録・削除・状態確認などを行う。
* **CLI**：コマンドラインからLWSを操作するためのインターフェース。本システムでは `lwsctl` を指す。
* **Backend**：Dashboardからの要求を処理し、アプリ管理、Docker操作、DNS・Reverse Proxy設定などの内部処理を担当するサーバー側コンポーネント。
* **SDK**：LWSに適合するWebアプリを外部で開発するためのTypeScriptパッケージ。LWSアプリ仕様に関する型定義、検証、ヘルパーなどを提供する。
* **Packaging**：LWSを `.deb` / `.rpm` として配布するためのパッケージ定義・生成処理。ファイル配置、メタデータ、ライフサイクルスクリプトなどを含む。
* **CI/CD**：テスト、ビルド、パッケージ生成、Dockerイメージ生成および各配布先への公開を自動化する仕組み。本システムでは主にGitHub Actionsを使用する。
* **mise**：開発時に必要な言語ランタイムやツールのバージョン管理、および開発・テスト・ビルド等のタスク実行を統一するためのツール。

---

## 🔄 フロー1：開発 → テスト → デプロイ

### 1. 開発

GitHubリポジトリをcloneし、`mise` を通して必要な開発ツールを準備したうえで、Docker Composeを使ってローカル環境を起動します。

```text
git clone
↓
miseによる開発環境セットアップ
↓
miseによる開発タスク実行
↓
Docker Composeでローカル環境起動
↓
Dashboard / Backend / CLI / SDK等を開発
↓
localhost等で動作確認
```

開発時に必要な言語ランタイムやビルドツールなどは、可能な限り `mise` で管理します。

これにより、開発者ごとにホストへ個別のバージョンを直接インストールすることを避け、リポジトリ単位で開発環境を再現できるようにします。

また、開発・テスト・ビルド・パッケージ生成などの定型処理も `mise` のタスクとして定義し、開発者が内部の複雑なコマンド列を直接操作する必要を減らします。

例えば、概念的には次のようなタスクを用意します。

```text
mise run dev
mise run test
mise run build
mise run package
```

開発環境と本番環境は基本構成を共通化し、開発時に必要な差分だけをComposeの開発用設定で上書きします。

### 2. テスト

本番と同じDocker Compose構成をベースに、テスト環境で統合動作を確認します。

確認対象には以下を含めます。

* Dashboard
* Backend
* CLI
* SDK
* Docker操作
* DNS
* Reverse Proxy
* アプリ登録・削除
* 起動・停止

LAN固有のDNS/DHCP連携については通常の開発とは分離し、統合テスト時に確認します。

テストタスクについても、可能な限り `mise` 経由で統一して実行します。

### 3. デプロイ導線

モノレポから複数種類の成果物を公開するため、デプロイ導線は以下の2点によって明確に区別します。

* Git tagの名前空間
* GitHub Actions Workflow YAML

主なリリース系統は次の2つとします。

#### LWS本体リリース

LWS本体、Backend、Dashboardは同一のLWSリリースとして扱います。

タグは次の形式とします。

```text
lws-v<x.y.z>
```

例：

```text
lws-v1.2.0
```

対応するGitHub Actions Workflowを用意します。

```text
.github/workflows/release-lws.yml
```

処理の流れは次のとおりです。

```text
lws-v1.2.0 tag push
↓
release-lws.yml
↓
test
├─ CLI / package build
├─ .deb build
├─ .rpm build
├─ Backend Docker image build
├─ Dashboard Docker image build
├─ GHCR push
└─ GitHub Release作成
```

成果物は次のように分離します。

```text
GitHub Releases
├─ lws_x.y.z_amd64.deb
├─ lws-x.y.z.x86_64.rpm
└─ checksum

GHCR
├─ backend:x.y.z
├─ dashboard:x.y.z
└─ その他のLWS本体用Docker image
```

LWS本体、Backend、Dashboardのバージョンは同一リリース内で揃えます。

例えばLWS `v1.2.0` であれば、

```text
LWS package       1.2.0
lwsctl            1.2.0
compose.yaml      1.2.0
backend image     1.2.0
dashboard image   1.2.0
```

という対応関係にします。

`.deb` / `.rpm` にDockerイメージそのものは含めません。

パッケージには主に以下を含めます。

* `lwsctl`
* `compose.yaml`
* 設定テンプレート
* lifecycle用スクリプト
* 必要なメタデータ

#### SDKリリース

SDKは外部Webアプリから依存される公開インターフェースであるため、LWS本体とは独立したバージョンを持たせます。

タグは次の形式とします。

```text
sdk-v<x.y.z>
```

例：

```text
sdk-v0.4.0
```

対応するGitHub Actions Workflowを用意します。

```text
.github/workflows/release-sdk.yml
```

処理の流れは次のとおりです。

```text
sdk-v0.4.0 tag push
↓
release-sdk.yml
↓
SDK test
↓
TypeScript build
↓
package生成
↓
GitHub Packagesへpublish
```

SDKはGitHub Packages上のTypeScript/npm互換パッケージとして公開します。

LWS本体の更新によってSDKの公開インターフェースに変更がない場合、SDKのバージョンを更新する必要はありません。

### 4. 通常CI

通常のpushやPull Requestでは、リリース処理を行わず、専用のCI Workflowでテストやビルド確認のみを行います。

例えば、

```text
.github/workflows/ci.yml
```

を用意し、

```text
push / Pull Request
↓
ci.yml
├─ lint
├─ test
├─ build check
└─ integration check
```

とします。

---

## 📦 フロー2：インストール → 利用

### 1. インストール

利用者はGitHub上に用意したインストールスクリプトを実行します。

```text
install.sh
↓
OS判定
↓
CPU architecture判定
↓
GitHub Releasesから適切な
.deb / .rpmを取得
↓
APT / DNFへ渡す
```

Ubuntu系では、

```text
apt install ./lws_x.y.z_amd64.deb
```

AlmaLinux系では、

```text
dnf install ./lws-x.y.z.x86_64.rpm
```

相当の処理を行います。

### 2. パッケージインストール

APT/DNFが以下を適切な場所へ配置します。

* `lwsctl`
* Compose定義
* 設定テンプレート
* 必要なシステムファイル

この段階ではDockerコンテナを起動しません。

```text
package install
↓
ファイル配置のみ
↓
LWSは停止状態
```

### 3. 初回起動

初期化専用コマンドは設けず、`lwsctl start` に初回セットアップを統合します。

```text
sudo lwsctl start
```

実行時に設定ファイルの有無を確認します。

```text
lwsctl start
↓
設定ファイル確認
├─ 存在する
│   └─ 設定を読み込んで起動
│
└─ 存在しない
    ↓
    初回起動処理
    ↓
    ベースドメインを入力
    ↓
    設定ファイル生成
    ↓
    Docker Compose起動
```

初回起動では、Webアプリ公開に使用するベースドメインを対話的に指定します。

コマンドラインから指定する場合は、`--domain` または `-d` を使用します。

```text
sudo lwsctl start --domain example.internal
sudo lwsctl start -d example.internal
```

### 4. ベースドメインの変更

設定済み環境で現在と異なるベースドメインを指定した場合は、上書き確認を行います。

```text
sudo lwsctl start -d new.internal
```

例：

```text
Current domain: old.internal
New domain:     new.internal

Changing the base domain will update application URLs
and DNS / reverse proxy settings.

Continue? [y/N]:
```

`yes` の場合のみ設定を更新します。

確認を省略して強制的に変更する場合は、`--force` または `-f` を使用します。

```text
sudo lwsctl start -d new.internal --force
sudo lwsctl start -d new.internal -f
```

変更後は以下の処理を行います。

```text
設定更新
↓
DNS設定再生成
↓
Reverse Proxy設定再生成
↓
必要なサービスを再構成
↓
起動
```

### 5. アプリの公開方式

設定したベースドメインを基準に、Dashboardおよび各Webアプリをサブドメインとして公開します。

例えばベースドメインが、

```text
example.internal
```

の場合、

```text
dashboard.example.internal
app-a.example.internal
app-b.example.internal
```

のように公開します。

基本形式は次のとおりです。

```text
<app-name>.<base-domain>
```

### 6. 通常起動

設定済みの場合は、

```text
sudo lwsctl start
```

でそのままLWSを起動します。

```text
lwsctl start
↓
設定確認
↓
Docker Compose
↓
GHCRから必要なimageをpull
↓
Dashboard
Backend
DNS
Reverse Proxy
等を起動
```

### 7. Webアプリの利用

Dashboardから、LWSに対応した形式で作成されたGitHubリポジトリを登録します。

```text
Dashboard
↓
GitHub Repository指定
↓
アプリ定義取得
↓
Docker Compose起動
↓
サブドメイン決定
↓
DNS登録
↓
Reverse Proxy登録
↓
LAN内からアクセス
```

Webアプリ自体のソースコードもGitHubで管理します。

外部でLWS対応Webアプリを開発する場合は、GitHub Packagesで公開されたLWS SDKを利用できます。

SDKはLWSへの適合に必要な仕様をTypeScriptから扱いやすくするための補助ライブラリとして位置付け、Webアプリそのものの実行をSDKへ依存させることは避けます。

### 8. システム更新

更新は `lwsctl` から実行します。

```text
sudo lwsctl update
```

内部では以下の処理を行います。

```text
GitHub Releases確認
↓
最新版の.deb / .rpm取得
↓
APT / DNFへ更新を委譲
↓
新しいCompose定義へ更新
↓
GHCRから対応する新しいimageをpull
↓
必要な再構築・再起動
```

パッケージの展開やファイル上書き自体は `lwsctl` で独自実装せず、APT/DNFに委譲します。

---

## 🗑️ フロー3：実行環境の撤去とパッケージ削除

`lwsctl down`はLWSの実行環境を撤去します。パッケージ削除はAPT/DNFの責務とし、パッケージだけ削除されてLWS管理下のDockerコンテナが動き続ける状態は、削除前hookで防ぎます。

### 1. 通常の実行環境撤去

ユーザー向けには、

```text
sudo lwsctl down
```

を用意します。

実行時には以下の処理を行います。

```text
Runtime状態確認
↓
LWS管理下コンテナ停止
↓
LWS管理下コンテナ削除
↓
LWS管理下network削除
```

通常の実行環境撤去では、再起動・再構成可能性を考慮して永続データを保持します。

```text
削除
├─ Dashboard / Backend等のコンテナ
├─ LWS管理下network

保持
├─ 永続データ
├─ Docker volume
├─ 必要なユーザー状態
└─ lwsctl / Compose定義などのパッケージ管理ファイル
```

### 2. 完全削除

完全に環境をクリーンにする場合は、

```text
sudo lwsctl down --purge
```

を使用します。

この場合は以下も削除対象とします。

* LWS設定
* LWS自身の状態データ
* LWS管理下の永続データ
* Docker volume

破壊的な操作であるため、CLI上で削除対象を明示して確認を行います。

確認を省略する場合は、`--force` または `-f` を使用します。

```text
sudo lwsctl down --purge --force
sudo lwsctl down --purge -f
```

### 3. パッケージ削除と安全網

パッケージを削除する場合は、APT/DNFを使用します。

```text
sudo apt remove lws
```

または、

```text
sudo dnf remove lws
```

を直接実行した場合にも、安全に削除できるようにします。

パッケージの削除前スクリプトから`lwsctl down`を呼び出します。

```text
package remove
↓
削除前hook
↓
LWS runtime状態確認
↓
安全にdown
↓
package削除
```

すでに停止済みの場合にも正常終了する冪等な処理とします。

---

## 🏗️ 最終的な責務分担

```text
GitHub Repository
├─ Source
├─ Docs
├─ CLI
├─ Dashboard
├─ Backend
├─ SDK
├─ Infrastructure
├─ Packaging
└─ CI/CD

mise
├─ 開発ツール・ランタイムのバージョン管理
└─ 開発 / テスト / ビルド等のタスクランナー

GitHub Actions
├─ CI
├─ LWS Release
└─ SDK Release

Git Tags
├─ lws-v* → LWS本体リリース
└─ sdk-v* → SDKリリース

GitHub Releases
└─ .deb / .rpm / checksum

GHCR
├─ Backend image
├─ Dashboard image
└─ その他のLWS本体用Docker images

GitHub Packages
└─ TypeScript SDK

APT / DNF
├─ Install
├─ Remove
├─ Package files
└─ OS側のpackage lifecycle

lwsctl
├─ start
├─ stop
├─ status
├─ rebuild
├─ update
└─ down

Docker Compose
└─ LWSおよびWebアプリのランタイム
```

LWSでは、「開発環境とタスク実行は `mise`」「Linuxへのインストールと所属はAPT/DNF」「LWS固有の操作は `lwsctl`」「実行環境はDocker Compose」「LWSパッケージ配布はGitHub Releases」「Dockerイメージ配布はGHCR」「SDK配布はGitHub Packages」という責務分担で統一します。

また、モノレポ内の複数のデプロイ導線は、Git tagの名前空間とGitHub Actions Workflowの分離によって明示的に区別します。
