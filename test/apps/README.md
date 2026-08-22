# テストアプリ

このディレクトリは、LWSの機能テストで使用するアプリsourceを管理する。

テストアプリは通常のLWS対応アプリと同じ構成にし、最低限次のファイルを持つ。

```text
compose.yaml
lws.manifest.yaml
```

fixtureは直接LWSへ渡さない。`scripts/test-app-fixture.sh`で一時Git repositoryへ変換し、テスト時だけGitのURL書き換えを設定する。LWSには本番と同じGitHub形式のURLを渡し、通常のsource取得処理を通して検証する。

fixtureの追加・変更では、次を守る。

- source root外を参照しない
- bind mount、host port、Docker socketを使わない
- 公開serviceからHTTP応答を返せる最小構成にする
- アプリ固有のテスト用分岐をLWS本体へ追加しない
- 正常系と失敗系のfixtureを別ディレクトリで管理する

## 現在のfixture

`valid/`は、nginxの`web` serviceを公開する最小の正常系アプリである。
