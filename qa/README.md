# LWS 機能受け入れテスト

このディレクトリは、LWS本体の内部実装ではなく、利用者が観測できる操作と結果を検証するためのテストを管理する。

テスト定義は`.robot`ファイルを正本とする。`backend/`や`cmd/`へ機能テスト用コードを追加しない。

## 実行

```sh
mise run qa
```

全機能テストを実行する。未実装項目は意図的にFAILになるため、これは現在のプロジェクト進捗を確認する入口である。実行結果は`test/result/robot/`へ生成される。

CIなど、現在実装済みの項目だけを確認する場合は次を実行する。

```sh
mise run qa-current
```

Robot Frameworkは`output.xml`、`log.html`、`report.html`を生成する。

## ドキュメント生成

```sh
mise run qa-docs
```

`qa/suites/`からTestdocでHTMLとJSONのドキュメントを生成し、`docs/generated/`へ出力する。

Robot Framework標準のTestdocはブラウザ側で描画するHTMLを生成するため、静的HTMLとJSONを生成できる`robotframework-testdoc`を使用する。
