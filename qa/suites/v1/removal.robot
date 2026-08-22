*** Settings ***
Documentation    v1の登録解除・再登録・完全削除受け入れテスト
Resource         ../../resources/v1.resource

*** Test Cases ***
FT-V1-040 ACTIVEアプリをunregisterする
    [Documentation]    container・edge network・公開設定を除去しsource・runtime・設定・volume・記録を保持する。
    [Tags]    FT-V1-040    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-040    ACTIVEアプリをunregister    実行resourceを除去し保存データを保持する

FT-V1-041 unregister失敗後に再試行可能にする
    [Documentation]    stop・切断・network削除の失敗時に既存記録を失わず再試行可能にする。
    [Tags]    FT-V1-041    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-041    unregister途中の操作を失敗させる    記録を保持し再試行可能にする

FT-V1-042 UNREGISTEREDアプリを再登録する
    [Documentation]    保持したsource・設定・volumeを再利用してACTIVEへ復帰する。
    [Tags]    FT-V1-042    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-042    UNREGISTEREDアプリをregister    保存データを再利用してACTIVEへ復帰する

FT-V1-043 ACTIVEアプリのpurgeを拒否する
    [Documentation]    ACTIVEアプリを完全削除せず拒否する。
    [Tags]    FT-V1-043    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-043    ACTIVEアプリをpurge    完全削除せず拒否する

FT-V1-044 未確認・所有不一致のpurgeを拒否する
    [Documentation]    confirmなし、所有不一致、installation ID不一致では削除を実行しない。
    [Tags]    FT-V1-044    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-044    不正条件でpurge    削除を実行せず拒否する

FT-V1-045 確認済みUNREGISTEREDアプリをpurgeする
    [Documentation]    LWS所有resourceとDB記録だけを削除する。
    [Tags]    FT-V1-045    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-045    確認済みUNREGISTEREDアプリをpurge    所有resourceと記録だけを削除する
