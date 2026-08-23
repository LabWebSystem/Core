# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

主な利用者は、研究室または家庭内LANを管理する人である。利用者は専門的なDocker・DNS・Reverse Proxy操作をせず、LAN内で使うWebアプリを登録、公開、運用する。

## Product Purpose

LabWebSystem（LWS）は、GitHubリポジトリのCompose定義からWebアプリをLAN内へ公開・管理できるようにする。アプリ単位のURLを提供し、管理者が日本語で、誰でも簡単に非専門的な操作で運用できることを目指す。

## Positioning

LWSは、GitHubリポジトリのCompose定義を検証してDocker Composeで実行し、アプリごとのLAN URL、DNS、Reverse Proxyを一貫して管理する。非専門家が日本語の操作画面を通じて利用できることが、一般的なDocker操作や手作業のプロキシ設定と異なる点である。

## Operating Context

信頼できる研究室・家庭内LANで利用する。管理者はGitHubリポジトリをアプリとして登録し、起動、停止、更新、設定値・secret・ログ・実行状態を管理する。アプリは`<app-name>.<base-domain>`形式のURLで公開する。

## Capabilities and Constraints

- Docker ComposeがLWSおよび登録アプリの実行基盤である。
- Backend APIがアプリ登録、Docker操作、検証、DNS・Reverse Proxy設定、状態永続化を担う。DashboardはBackend API経由で操作する。
- GitHubリポジトリURL、アプリ定義、アプリ名、ベースドメイン、APIリクエストは未信頼入力として扱う。
- 現段階では、重厚な認証機能は実装しない。
- 管理APIは認証・認可未実装のため、信頼できるLAN内だけで利用する。
- DashboardとTypeScript SDKは開発途中である。

## Brand Commitments

- ユーザー向けの文章は日本語を正本とする。
- アイコンとアニメーションを効果的に用い、言語的な指示・説明は最小限にする。
- カードを多用しない、シンプルでモダンなフラットデザインを採用する。
- 軽快な操作性と低遅延を重視する。

## Evidence on Hand

実装・仕様・利用手順は、`README.md`、`docs/`、`backend/`、`cmd/lwsctl/`、`infrastructure/`にある。Dashboardは現在Dockerfileとモックサーバーのみで、完成した画面実装やデザイン資産はない。実在の顧客事例、導入実績、評価値は確認できていないため、将来の画面や文書で創作しない。

## Product Principles

1. 専門的なインフラ操作を、安心して実行できる日本語の管理体験へ置き換える。
2. アプリ公開に必要な状態を一貫して扱い、管理者に中途半端な設定を残さない。
3. LAN内利用でも、未信頼入力の検証とLWS所有リソースの保護を省略しない。
4. 日常の管理操作は速く、理解と判断に必要な情報だけを明確に示す。
5. まず全機能を扱える簡素なデバッグルームを成立させ、機能・導線の検証を優先してから段階的に画面を洗練する。
