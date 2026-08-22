*** Settings ***
Documentation    v1のLWS本体ライフサイクル受け入れテスト
Resource         ../../resources/lifecycle.resource
Suite Setup      LWS本体の隔離環境を準備する
Suite Teardown   LWS本体を停止して隔離環境を削除する
Test Setup       LWSライフサイクル環境が利用可能であること

*** Test Cases ***
FT-V1-004 初回startでLWSを起動する
    [Documentation]    隔離環境へ実際にlwsctl startを実行し、設定とComposeの起動を確認する。実行には必須ポートをbindできる権限が必要。
    [Tags]    FT-V1-004    planned    lifecycle    v1
    LWSを起動する
    LWSが起動済みであることを確認する

FT-V1-005 不正domainでstartを拒否する
    [Documentation]    不正domainでは設定とComposeを変更せず失敗する。
    [Tags]    FT-V1-005    planned    lifecycle    v1
    ${result}=    Run Process    ${LWSCTL}    start    --domain    invalid_domain
    Should Not Be Equal As Integers    ${result.rc}    0
    File Should Exist    ${QA_ROOT}/etc/config.env
    ${config}=    Get File    ${QA_ROOT}/etc/config.env
    Should Contain    ${config}    LWS_BASE_DOMAIN=qa.example.internal

FT-V1-006 start済みLWSをstop/status/rebuildする
    [Documentation]    stop、status、rebuildが対象だけを操作し設定を保持する。
    [Tags]    FT-V1-006    planned    lifecycle    v1
    ${stop}=    Run Process    ${LWSCTL}    stop
    Log    stdout=${stop.stdout} stderr=${stop.stderr}
    Should Be Equal As Integers    ${stop.rc}    0
    ${status}=    Run Process    ${LWSCTL}    status
    Should Be Equal As Integers    ${status.rc}    0
    ${rebuild}=    Run Process    ${LWSCTL}    rebuild
    Log    stdout=${rebuild.stdout} stderr=${rebuild.stderr}
    Should Be Equal As Integers    ${rebuild.rc}    0
    File Should Exist    ${QA_ROOT}/etc/config.env
