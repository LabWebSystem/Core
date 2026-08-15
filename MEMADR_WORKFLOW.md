# MEMADR_WORKFLOW.md

## 目的

このリポジトリでは、MemADRを開発判断メモリとして使用する。

MemADRは、作業ログを保存するための仕組みではない。  
MemADRは、次回以降の開発判断に必要な「問題」「原因」「設計判断」「変更」「解決策」「無効化された前提」を、短いMarkdownレコードとして管理するために使用する。

特に、このリポジトリでは次を重視する。

- 過去には有効だった情報と、現在も判断に使える情報を区別する
- 削除済み機能、廃止済み仕様、置き換え済み設計を、現在有効な前提として扱わない
- 不要になった情報は削除せず、現在価値を下げた状態として明示する
- エージェントが古い情報を誤って参照しないようにする

---

## 基本方針

- `memory/` 配下のMarkdownレコードを、開発判断メモリの正本として扱う。
- `memory/generated/` 配下は生成物として扱い、手動編集しない。
- 作業ログ、試行錯誤、逐次的な実行履歴は `memory/` に書かない。
- 記録するのは、将来の判断に影響する事実、原因、判断、変更、検証結果、無効化情報に限る。
- レコードは短く、具体的に書く。
- 古い情報を削除せず、現在価値を下げる場合は `SUP`、`SUPERSEDED`、`ARCHIVED`、`FutureRelevance` を使って明示する。
- 不確かなことを、確定した判断として記録しない。
- コード、テスト、MemADRレコードの内容が矛盾する場合は、コードとテストを確認したうえで、MemADRレコードを更新する。

---

## 作業開始時の確認

作業を始める前に、可能な範囲で次を確認する。

````bash
memadr index
memadr list --status OPEN
memadr list --status UNRESOLVED
memadr list --status ACCEPTED
memadr list --status ACTIVE
````

対象領域が分かっている場合は、関連語で検索する。

````bash
memadr search "<対象機能名>"
memadr search "<対象ディレクトリ名>"
memadr search "<関連する概念名>"
````

既存レコードが見つかった場合は、必要に応じて関連レコードを確認する。

````bash
memadr related <RECORD-ID>
````

作業前に把握すべき内容は次の通り。

- 未解決の問題
- 現在有効な設計判断
- 現在有効な解決策
- 今回触る領域の過去のバグ
- 今回触る領域で無効化済みの前提
- 過去には存在したが、現在は削除済みまたは非推奨の機能

---

## MemADRレコードの種別

### BUG

バグ、不具合、失敗を記録する。

使用する場面:

- 実際に発生した不具合を修正する場合
- 再発可能性のある失敗を見つけた場合
- 原因が将来の判断に影響する場合

記録しないもの:

- 一時的な作業ミス
- すぐ破棄した試行錯誤
- 将来参照する価値がない単発の失敗

推奨フィールド:

````md
Status:
Area:
Symptom:
Cause:
Fix:
Verification:
FutureRelevance:
Decision:
ResolvedBy:
Related:
CurrentValue:
ValidityScope:
````

---

### PROB

構造的な問題や未解決課題を記録する。

使用する場面:

- 単独のバグではなく、設計上の歪みがある場合
- 今回すぐに解決できないが、将来の作業に影響する場合
- 複数のBUGやCHGの背景に同じ問題がある場合

---

### ADR

設計判断を記録する。

使用する場面:

- 方針を選んだ場合
- 複数案から一つを採用した場合
- 今後の実装を制約する判断を行った場合
- 採用しなかった案も将来誤って復活しそうな場合

推奨フィールド:

````md
Status:
Context:
Decision:
Rejected:
Consequence:
Related:
CurrentValue:
ValidityScope:
````

---

### CHG

実際に行った変更を記録する。

使用する場面:

- 設計判断に基づいて実装を変更した場合
- バグ修正を行った場合
- 機能追加、機能削除、構成変更を行った場合
- 将来の差分理解に必要な変更を行った場合

推奨フィールド:

````md
Status:
Reason:
Change:
FilesChanged:
Verification:
Related:
CurrentValue:
ValidityScope:
````

---

### REV

過去の変更を戻した場合に記録する。

使用する場面:

- 変更をロールバックした場合
- 採用した実装方針を撤回した場合
- 一時的に戻したが、再導入条件を残す必要がある場合

---

### SOL

再利用可能な解決策を記録する。

使用する場面:

- 同種の問題に再利用できる修正パターンを得た場合
- 今後の実装で守るべき定石ができた場合
- バグ修正から一般化可能な知見が得られた場合

`SOL` は原則として `FutureRelevance: reusable` とする。

---

### SUP

過去の判断、実装、変更、バグ情報を無効化または置き換える場合に記録する。

使用する場面:

- 機能削除により、過去のバグ修正情報の現在価値がなくなった場合
- 後続のADRにより、過去のADRが置き換えられた場合
- 古い実装方針を今後使ってはならない場合
- 過去のBUG、CHG、SOLが最新バージョンでは参照不要になった場合

推奨フィールド:

````md
Status: ACTIVE
Supersedes:
Reason:
NewBaseline:
CurrentValue:
ValidityScope:
ReactivatedIf:
Related:
````

---

## 現在価値の管理

このリポジトリでは、過去情報を単に残すだけでなく、その情報が現在の開発判断に使えるかを管理する。

### CurrentValue

`CurrentValue` は、このリポジトリで使用する補助フィールドである。

使用可能な値:

| 値 | 意味 |
|---|---|
| `current` | 現在の開発判断に使用する |
| `watch` | 関連領域では注意して参照する |
| `historical` | 履歴としてのみ保持する |
| `none` | 通常の開発判断では参照しない |

### FutureRelevance

`FutureRelevance` は、将来の参照価値を表す。

使用可能な値:

| 値 | 意味 |
|---|---|
| `ignore` | 解決済みで通常は考慮不要 |
| `watch` | 再発や副作用に注意 |
| `reusable` | 同種問題の解決策として再利用価値がある |

### CurrentValueとFutureRelevanceの使い分け

| 状況 | CurrentValue | FutureRelevance |
|---|---|---|
| 現在も設計判断に使う | `current` | `watch` または `reusable` |
| 現在は使わないが再発に注意 | `watch` | `watch` |
| 履歴としてのみ残す | `historical` | `ignore` |
| 機能削除などで通常参照不要 | `none` | `ignore` |
| 同種問題へ再利用できる | `current` または `watch` | `reusable` |

---

## 古い情報を扱う規則

過去のBUG、CHG、ADR、SOLが現在の開発判断に不要になった場合、削除ではなく無効化または価値低下として扱う。

### 機能が削除された場合

例:

- 過去バージョンにCSVインポート機能が存在した
- CSVインポート機能にBUGがあった
- BUGはCHGで修正済みだった
- 最新バージョンでCSVインポート機能自体を削除した

この場合の扱い:

1. 機能削除を `CHG` として記録する。
2. 削除により現在価値がなくなった過去レコードを `SUP` に列挙する。
3. 元の `BUG` や `CHG` は `Status: ARCHIVED` または `Status: SUPERSEDED` にする。
4. 元の `BUG` や `CHG` に `FutureRelevance: ignore` を設定する。
5. 復活時にのみ参照すべき条件を `ReactivatedIf` に書く。

例:

````md
# SUP-003: CSVインポート関連の過去バグ修正情報を通常参照から外す

Status: ACTIVE
Supersedes: BUG-012, CHG-021
Reason: CSVインポート機能自体をCHG-045で削除したため、当該バグ修正情報は最新バージョンの実装判断には不要になった
NewBaseline: CHG-045
CurrentValue: current
ValidityScope: latest
ReactivatedIf: CSVインポート機能を再導入する場合のみ、BUG-012とCHG-021を履歴として確認する
Related: CHG-045
````

元のBUG側は次のように更新する。

````md
# BUG-012: CSVインポート時に列順が崩れる

Status: ARCHIVED
Area: import
Symptom: CSVインポート時に特定条件で列順が崩れる
Cause: ヘッダー名ではなく列番号でマッピングしていた
Fix: ヘッダー名ベースのマッピングに変更した
Verification: 複数列順のCSVで確認済み
FutureRelevance: ignore
CurrentValue: none
ValidityScope: removed-feature
ResolvedBy: CHG-021
Related: CHG-045, SUP-003
````

---

## レコードの参照優先度

作業時にレコードを参照する優先度は次の通り。

### 優先して読む

- `Status: OPEN`
- `Status: UNRESOLVED`
- `Status: ACCEPTED`
- `Status: ACTIVE`
- `CurrentValue: current`
- `FutureRelevance: reusable`
- 今回の対象領域に一致する `FutureRelevance: watch`

### 条件付きで読む

- `Status: VERIFIED`
- `Status: SHIPPED`
- `CurrentValue: watch`
- 今回の対象領域に関係する過去BUG
- 今回の対象領域に関係する過去CHG

### 原則として読まない

- `Status: ARCHIVED`
- `CurrentValue: historical`
- `CurrentValue: none`
- `FutureRelevance: ignore`

### 例外的に読む

次の場合は、`ARCHIVED` や `ignore` のレコードも確認する。

- 削除済み機能を復活させる場合
- 過去バージョンの挙動を調査する場合
- 回帰バグの原因が過去実装に関係する場合
- `SUP` の `ReactivatedIf` 条件に該当する場合
- ユーザーが明示的に過去情報の確認を求めた場合

---

## 作業中にレコードを作る判断基準

次に該当する場合は、MemADRレコードを作成または更新する。

| 状況 | 作成・更新する種別 |
|---|---|
| バグを見つけた | `BUG` |
| バグの背後に構造問題がある | `PROB` |
| 設計方針を決めた | `ADR` |
| 実装を変更した | `CHG` |
| 変更を戻した | `REV` |
| 再利用可能な解決策ができた | `SOL` |
| 古い判断や変更を無効化した | `SUP` |
| 機能削除により過去BUGの価値が消えた | `CHG` と `SUP` |
| 過去ADRが新ADRに置き換わった | `ADR` と `SUP` |
| 過去情報を通常参照から外す | `SUP`、元レコードの `ARCHIVED` 化 |

---

## 作業完了前の確認

MemADRレコードを追加または更新した場合は、完了前に必ず次を実行する。

````bash
memadr check
memadr index
````

`memadr check` が失敗した場合は、レコードの形式、ID、Status、FutureRelevance、参照関係を修正する。

`memory/generated/` 配下の差分は、`memadr index` による生成結果として扱う。

---

## エージェントの回答方針

作業完了時には、以下を簡潔に報告する。

- 実装した変更
- 実行した検証
- 追加または更新したMemADRレコード
- 無効化した過去レコード
- 現在価値を変更したレコード
- 残っている未解決事項

MemADRレコードを作らなかった場合は、作らなかった理由を簡潔に示す。

例:

````md
MemADRレコードは追加していません。今回の変更は表記修正のみであり、将来の開発判断に影響する設計判断、バグ原因、再利用可能な解決策、無効化情報が発生していないためです。
````

---

## 禁止事項

- `memory/` に作業ログを書くこと。
- `memory/generated/` を手動編集すること。
- 古いBUGやCHGを、現在も有効な制約として無条件に扱うこと。
- `SUP` で無効化されたADR、CHG、BUGを、新しい基準より優先すること。
- 検証していない修正を `VERIFIED` とすること。
- 将来参照価値のない軽微な作業を、MemADRレコードとして大量に残すこと。
- レコード間の参照関係を確認せずに、新しいADRやSUPを作ること。
- 現在価値が下がった情報を放置し、エージェントが誤参照する状態にすること。

---

## 最重要ルール

このリポジトリでは、過去情報を消すのではなく、現在の判断対象から外す。

不要になった情報は削除せず、次のいずれかで明示する。

- `Status: ARCHIVED`
- `Status: SUPERSEDED`
- `FutureRelevance: ignore`
- `CurrentValue: historical`
- `CurrentValue: none`
- `SUP` による無効化または置き換え

エージェントは、常に「この情報は現在も判断に使えるか」を確認してから実装する。