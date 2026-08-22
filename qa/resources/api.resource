*** Settings ***
Library    RequestsLibrary
Library    Collections
Library    OperatingSystem
Library    String

*** Keywords ***
APIの接続を設定する
    ${base}=    Get Environment Variable    LWS_QA_BASE_URL    http://127.0.0.1/api/v1
    ${domain}=    Get Environment Variable    LWS_QA_BASE_DOMAIN    example.internal
    ${headers}=    Create Dictionary    Host=api.${domain}
    Create Session    lws    ${base}    headers=${headers}
    Set Suite Variable    ${LWS_QA_BASE_URL}    ${base}

ヘルスチェックが成功する
    ${response}=    GET On Session    lws    /health/live    expected_status=200
    Should Be Equal As Strings    ${response.text}    OK

アプリ登録要求を送信する
    [Arguments]    ${request_id}    ${subdomain}=qa-app
    ${repository}=    Get Environment Variable    LWS_QA_REPOSITORY_URL    https://github.com/test/lws-valid
    ${ref}=    Get Environment Variable    LWS_QA_REPOSITORY_REF    main
    ${body}=    Create Dictionary    repositoryUrl=${repository}    ref=${ref}    subdomain=${subdomain}    requestId=${request_id}
    ${response}=    POST On Session    lws    /applications    json=${body}    expected_status=anything
    RETURN    ${response}

Operation名を取得する
    [Arguments]    ${response}
    Should Be Equal As Integers    ${response.status_code}    202
    ${body}=    Evaluate    $response.json()
    ${name}=    Get From Dictionary    ${body}    name
    Should Match Regexp    ${name}    ^operations/.+
    ${operation}=    Replace String    ${name}    operations/    ${EMPTY}
    RETURN    ${operation}

Operationが終端状態になるまで待つ
    [Arguments]    ${operation}
    Wait Until Keyword Succeeds    60 sec    1 sec    Operationの状態を確認する    ${operation}

Operationの状態を確認する
    [Arguments]    ${operation}
    ${response}=    GET On Session    lws    /operations/${operation}    expected_status=200
    ${body}=    Evaluate    $response.json()
    ${state}=    Get From Dictionary    ${body}    state
    IF    '${state}' in ['succeeded', 'failed', 'cancelled']
        RETURN    ${state}
    END
    Fail    Operationが終端状態になっていません: ${state}

Operationを再送して同じ名前を確認する
    [Arguments]    ${request_id}    ${subdomain}
    ${first}=    アプリ登録要求を送信する    ${request_id}    ${subdomain}
    ${first_body}=    Evaluate    $first.json()
    ${first_name}=    Get From Dictionary    ${first_body}    name
    ${second}=    アプリ登録要求を送信する    ${request_id}    ${subdomain}
    Should Be Equal As Integers    ${second.status_code}    202
    ${second_body}=    Evaluate    $second.json()
    ${second_name}=    Get From Dictionary    ${second_body}    name
    Should Be Equal As Strings    ${second_name}    ${first_name}

アプリ操作を実行して成功を待つ
    [Arguments]    ${path}    ${request_id}
    ${body}=    Create Dictionary    requestId=${request_id}
    ${response}=    POST On Session    lws    ${path}    json=${body}    expected_status=202
    ${operation}=    Operation名を取得する    ${response}
    ${state}=    Operationが終端状態になるまで待つ    ${operation}
    Should Be Equal As Strings    ${state}    succeeded

設定操作を実行して成功を待つ
    [Arguments]    ${application}    ${variables}    ${request_id}
    ${body}=    Create Dictionary    variables=${variables}    requestId=${request_id}
    ${response}=    PATCH On Session    lws    /applications/${application}/configuration    json=${body}    expected_status=202
    ${operation}=    Operation名を取得する    ${response}
    ${state}=    Operationが終端状態になるまで待つ    ${operation}
    Should Be Equal As Strings    ${state}    succeeded

未実装APIを失敗させる
    [Arguments]    ${id}    ${reason}
    Fail    ${id}: APIレベルで未実装です。${reason}
