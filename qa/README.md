# LWS 機能受け入れテスト

このディレクトリは、LWS本体の内部実装ではなく、利用者が観測できる操作と結果を検証するためのテストを管理する。

テスト定義は`.robot`ファイルを正本とする。`backend/`や`cmd/`へ機能テスト用コードを追加しない。

## 実行

```sh
mise run qa
```

通常の機能テストを実行する。外部で起動済みのLWS APIを必要とする`live`タグは対象外で、`mise run qa-live`で明示的に実行する。未達成項目はFAILとして結果へ記録するが、Robotの実行自体が最後まで完了した場合は`mise`の終了statusを成功にする。実行結果は`test/result/robot/`へ生成される。インストールテストはrootで起動する一時Dockerコンテナ内で実行し、コンテナはテスト後に自動削除する。

CIなど、現在実装済みの項目だけを確認する場合は次を実行する。

```sh
mise run qa-current
```

Robot Frameworkは`output.xml`、`log.html`、`report.html`を生成する。

起動済みLWSのCaddy公開経路へ接続してAPIレベルの受け入れテストを実行する場合は、`LWS_QA_BASE_URL`と`LWS_QA_BASE_DOMAIN`などを設定して次を実行する。Backend内部の8080/tcpへは直接接続しない。

```sh
LWS_QA_BASE_URL=http://127.0.0.1/api/v1 \
LWS_QA_BASE_DOMAIN=example.internal \
LWS_QA_REPOSITORY_URL=https://github.com/example/lws-valid \
mise run qa-live
```

`qa-live`はLWSを自動起動しない。LWSが起動していない場合はテストをSKIPせず、接続エラーとしてFAILする。

LWSの起動・停止・再構成を実際のDocker Composeで確認する場合は、専用の隔離環境を使って次を実行する。

```sh
mise run qa-lifecycle
```

このテストはホストのDockerと80/tcp、53/tcp、53/udpを使用するため、専用のテスト環境で実行する。結果は`test/result/robot-lifecycle/`へ出力される。

## ドキュメント生成

```sh
mise run qa-docs
```

`qa/suites/`からTestdocでHTMLとJSONのドキュメントを生成し、`docs/generated/`へ出力する。

Robot Framework標準のTestdocはブラウザ側で描画するHTMLを生成するため、静的HTMLとJSONを生成できる`robotframework-testdoc`を使用する。
