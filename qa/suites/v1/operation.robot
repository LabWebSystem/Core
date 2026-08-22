*** Settings ***
Documentation    v1のOperation・SSE・障害復旧受け入れテスト
Resource         ../../resources/v1.resource

*** Test Cases ***
FT-V1-050 同じrequestIdの変更を冪等に再送する
    [Documentation]    同じOperationを返し副作用を重複実行しない。
    [Tags]    FT-V1-050    planned    contract    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-050    同じrequestIdで同じ変更を再送    同じOperationを返し副作用を重複させない

FT-V1-051 requestIdの異なる内容を拒否する
    [Documentation]    同じrequestIdで異なる内容を送信した場合は副作用なしで拒否する。
    [Tags]    FT-V1-051    planned    contract    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-051    同じrequestIdで異なる変更を送信    副作用なしで拒否する

FT-V1-052 同じアプリへの並行変更を排他する
    [Documentation]    一方を実行し、他方を409で拒否する。
    [Tags]    FT-V1-052    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-052    同じアプリへ並行変更を送信    一方を実行し他方を409で拒否する

FT-V1-053 OperationをSSE購読する
    [Documentation]    envelope、状態遷移、終端状態を受信する。
    [Tags]    FT-V1-053    planned    contract    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-053    OperationをSSE購読    状態遷移と終端状態を受信する

FT-V1-054 SSE切断後に状態を復元する
    [Documentation]    再接続時にHTTP再取得またはsnapshotで欠損状態を復元する。
    [Tags]    FT-V1-054    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-054    SSE接続を切断して再接続    欠損状態を復元する

FT-V1-055 低速subscriberでworkerを停止させない
    [Documentation]    低速SSE subscriberとログ購読がworker・publisherを停止させずbufferを無制限に増やさない。
    [Tags]    FT-V1-055    planned    component    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-055    低速SSE subscriberとログを購読    workerを停止させずbufferを制限する

FT-V1-056 所有確認済みcontainerのログだけを購読する
    [Documentation]    所有確認済みComposeのログだけをSSEで配信する。
    [Tags]    FT-V1-056    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-056    コンテナログを購読    所有済みログだけを配信する

FT-V1-057 Backend再起動後に未完了Operationを整理する
    [Documentation]    未完了Operationをfailedへ整理しSQLiteからDocker・派生設定を再調整する。
    [Tags]    FT-V1-057    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-057    BackendをOperation実行中に再起動    Operationを整理しruntimeを再調整する

FT-V1-058 障害時に有効状態を保護する
    [Documentation]    DB・Docker・network・Caddy・CoreDNS障害時にOperationを失敗させ既存状態を不要に変更しない。
    [Tags]    FT-V1-058    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-058    各基盤障害を発生させる    既存の有効状態を保護する
