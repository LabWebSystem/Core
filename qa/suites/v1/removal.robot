*** Settings ***
Documentation    v1の登録解除・再登録・完全削除受け入れテスト
Resource         ../../resources/v1.resource
Resource         ../../resources/api.resource
Suite Setup      APIの接続を設定する

*** Test Cases ***
FT-V1-040 ACTIVEアプリをunregisterする
    [Documentation]    container・edge network・公開設定を除去しsource・runtime・設定・volume・記録を保持する。
    [Tags]    FT-V1-040    live    planned    workflow    v1
    ${application}=    Get Environment Variable    LWS_QA_APPLICATION    qa-app
    ${body}=    Create Dictionary    requestId=550e8400-e29b-41d4-a716-446655440240
    ${response}=    DELETE On Session    lws    /applications/${application}    json=${body}    expected_status=202
    ${operation}=    Operation名を取得する    ${response}
    ${state}=    Operationが終端状態になるまで待つ    ${operation}
    Should Be Equal As Strings    ${state}    succeeded
    ${current}=    GET On Session    lws    /applications/${application}    expected_status=200
    ${current_body}=    Evaluate    $current.json()
    ${registration}=    Get From Dictionary    ${current_body}    registrationState
    Should Be Equal As Strings    ${registration}    UNREGISTERED

FT-V1-041 unregister失敗後に再試行可能にする
    [Documentation]    stop・切断・network削除の失敗時に既存記録を失わず再試行可能にする。
    [Tags]    FT-V1-041    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-041    unregister途中の操作を失敗させる    記録を保持し再試行可能にする

FT-V1-042 UNREGISTEREDアプリを再登録する
    [Documentation]    保持したsource・設定・volumeを再利用してACTIVEへ復帰する。
    [Tags]    FT-V1-042    live    planned    workflow    v1
    ${application}=    Get Environment Variable    LWS_QA_APPLICATION    qa-app
    ${body}=    Create Dictionary    requestId=550e8400-e29b-41d4-a716-446655440242
    ${response}=    POST On Session    lws    /applications/${application}:register    json=${body}    expected_status=202
    ${operation}=    Operation名を取得する    ${response}
    ${state}=    Operationが終端状態になるまで待つ    ${operation}
    Should Be Equal As Strings    ${state}    succeeded
    ${current}=    GET On Session    lws    /applications/${application}    expected_status=200
    ${current_body}=    Evaluate    $current.json()
    ${registration}=    Get From Dictionary    ${current_body}    registrationState
    Should Be Equal As Strings    ${registration}    ACTIVE

FT-V1-043 ACTIVEアプリのpurgeを拒否する
    [Documentation]    ACTIVEアプリを完全削除せず拒否する。
    [Tags]    FT-V1-043    live    planned    workflow    v1
    ${application}=    Get Environment Variable    LWS_QA_APPLICATION    qa-app
    ${body}=    Create Dictionary    requestId=550e8400-e29b-41d4-a716-446655440243    confirm=${True}
    ${response}=    POST On Session    lws    /applications/${application}:purge    json=${body}    expected_status=anything
    Should Be Equal As Integers    ${response.status_code}    400

FT-V1-044 未確認・所有不一致のpurgeを拒否する
    [Documentation]    confirmなし、所有不一致、installation ID不一致では削除を実行しない。
    [Tags]    FT-V1-044    live    planned    workflow    v1
    ${application}=    Get Environment Variable    LWS_QA_APPLICATION    qa-app
    ${body}=    Create Dictionary    requestId=550e8400-e29b-41d4-a716-446655440244    confirm=${False}
    ${response}=    POST On Session    lws    /applications/${application}:purge    json=${body}    expected_status=anything
    Should Be Equal As Integers    ${response.status_code}    400

FT-V1-045 確認済みUNREGISTEREDアプリをpurgeする
    [Documentation]    LWS所有resourceとDB記録だけを削除する。
    [Tags]    FT-V1-045    live    planned    workflow    v1
    ${application}=    Get Environment Variable    LWS_QA_APPLICATION    qa-app
    ${unregister}=    Create Dictionary    requestId=550e8400-e29b-41d4-a716-446655440245
    ${unregister_response}=    DELETE On Session    lws    /applications/${application}    json=${unregister}    expected_status=202
    ${unregister_operation}=    Operation名を取得する    ${unregister_response}
    ${unregister_state}=    Operationが終端状態になるまで待つ    ${unregister_operation}
    Should Be Equal As Strings    ${unregister_state}    succeeded
    ${body}=    Create Dictionary    requestId=550e8400-e29b-41d4-a716-446655440246    confirm=${True}
    ${response}=    POST On Session    lws    /applications/${application}:purge    json=${body}    expected_status=202
    ${operation}=    Operation名を取得する    ${response}
    ${state}=    Operationが終端状態になるまで待つ    ${operation}
    Should Be Equal As Strings    ${state}    succeeded
    ${current}=    GET On Session    lws    /applications/${application}    expected_status=anything
    Should Be Equal As Integers    ${current.status_code}    404
