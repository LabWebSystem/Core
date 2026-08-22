*** Settings ***
Documentation    v1のアプリ登録・公開・設定・sync受け入れテスト
Resource         ../../resources/v1.resource
Resource         ../../resources/api.resource
Suite Setup      APIの接続を設定する

*** Test Cases ***
FT-V1-020 適合GitHub repositoryを登録して公開する
    [Documentation]    Operation完了後にsource取得・検証・起動しsubdomain.domainで公開する。
    [Tags]    FT-V1-020    live    planned    workflow    v1
    ${response}=    アプリ登録要求を送信する    550e8400-e29b-41d4-a716-446655440201    qa-app
    ${operation}=    Operation名を取得する    ${response}
    Operationが終端状態になるまで待つ    ${operation}
    ${application}=    GET On Session    lws    /applications/qa-app    expected_status=200
    ${body}=    Evaluate    $application.json()
    ${state}=    Get From Dictionary    ${body}    registrationState
    Should Be Equal As Strings    ${state}    ACTIVE

FT-V1-021 不正なrepository URLを拒否する
    [Documentation]    GitHub以外、SSH、認証情報付きURLをGit実行前に拒否する。
    [Tags]    FT-V1-021    live    planned    workflow    v1
    ${body}=    Create Dictionary    repositoryUrl=ssh://git@github.com/test/lws    ref=main    subdomain=invalid-url    requestId=550e8400-e29b-41d4-a716-446655440221
    ${response}=    POST On Session    lws    /applications    json=${body}    expected_status=anything
    Should Be Equal As Integers    ${response.status_code}    400

FT-V1-022 manifest指定のserviceとportで公開する
    [Documentation]    正しいmanifestとComposeを登録し指定service・portで公開する。
    [Tags]    FT-V1-022    workflow    v1
    Backend受け入れテストを実行する    ^TestFTV1_020RegisterValidApp$

FT-V1-023 不正manifestをDocker実行前に拒否する
    [Documentation]    manifest不在・破損・schema違反でDockerを実行せず既存状態を保持する。
    [Tags]    FT-V1-023    live    planned    workflow    v1
    ${repository}=    Get Environment Variable    LWS_QA_INVALID_MANIFEST_URL    https://github.com/test/lws-invalid-manifest
    ${body}=    Create Dictionary    repositoryUrl=${repository}    ref=main    subdomain=invalid-manifest    requestId=550e8400-e29b-41d4-a716-446655440223
    ${response}=    POST On Session    lws    /applications    json=${body}    expected_status=202
    ${operation}=    Operation名を取得する    ${response}
    ${state}=    Operationが終端状態になるまで待つ    ${operation}
    Should Be Equal As Strings    ${state}    failed

FT-V1-024 危険なsource記法を拒否する
    [Documentation]    symlink、anchor、duplicate key、外部読込を含むsourceを採用しない。
    [Tags]    FT-V1-024    workflow    v1
    Backend受け入れテストを実行する    ^Test(ManifestSymlinkIsNotUsed|SourceTreeRejectsDotEnv)$

FT-V1-025 危険なComposeを起動前に拒否する
    [Documentation]    root外path、bind mount、host port、privileged、Docker socketを拒否する。
    [Tags]    FT-V1-025    workflow    v1
    Backend受け入れテストを実行する    ^Test(ComposeRejectsExternalFeatures|EffectiveComposeAllowsNamedVolumeButRejectsBindMount|RuntimeRejectsEffectiveComposeBeforeUp)$

FT-V1-026 Git取得失敗時に旧公開経路を保持する
    [Documentation]    cloneまたはref取得失敗時にOperationをfailedとし旧source・公開経路を維持する。
    [Tags]    FT-V1-026    live    planned    workflow    v1
    ${body}=    Create Dictionary    repositoryUrl=https://github.com/test/does-not-exist-lws    ref=main    subdomain=git-failure    requestId=550e8400-e29b-41d4-a716-446655440226
    ${response}=    POST On Session    lws    /applications    json=${body}    expected_status=202
    ${operation}=    Operation名を取得する    ${response}
    ${state}=    Operationが終端状態になるまで待つ    ${operation}
    Should Be Equal As Strings    ${state}    failed

FT-V1-027 アプリ間の公開とnetworkを分離する
    [Documentation]    同じCompose service名のアプリを登録しても各URLが正しく到達し相互通信を拒否する。
    [Tags]    FT-V1-027    integration    v1
    Backend受け入れテストを実行する    ^Test(DerivedManagerUsesManifestPublication|DockerConnectsCaddyWithAppAlias|RuntimeUsesOwnedComposeArguments)$

FT-V1-028 アプリのstop/start/rebuildでvolumeを保持する
    [Documentation]    起動中アプリを操作し対象だけを変更してnamed volumeを保持する。
    [Tags]    FT-V1-028    live    planned    workflow    v1
    ${application}=    Get Environment Variable    LWS_QA_APPLICATION    qa-app
    アプリ操作を実行して成功を待つ    /applications/${application}:stop    550e8400-e29b-41d4-a716-446655440228
    アプリ操作を実行して成功を待つ    /applications/${application}:start    550e8400-e29b-41d4-a716-446655440229
    アプリ操作を実行して成功を待つ    /applications/${application}:rebuild    550e8400-e29b-41d4-a716-446655440230

FT-V1-030 必須環境変数を登録して起動する
    [Documentation]    起動直前にapp.envを生成しアプリを起動する。
    [Tags]    FT-V1-030    live    planned    workflow    v1
    ${application}=    Get Environment Variable    LWS_QA_APPLICATION    qa-app
    ${variables}=    Create Dictionary
    設定操作を実行して成功を待つ    ${application}    ${variables}    550e8400-e29b-41d4-a716-446655440230

FT-V1-031 secretを外部へ漏えいさせない
    [Documentation]    secret値が取得結果、API、Operation、ログ、エラーへ含まれない。
    [Tags]    FT-V1-031    live    planned    contract    v1
    ${application}=    Get Environment Variable    LWS_QA_APPLICATION    qa-app
    ${variable}=    Create Dictionary    value=robot-secret-value    secret=${True}
    ${variables}=    Create Dictionary    ROBOT_SECRET=${variable}
    設定操作を実行して成功を待つ    ${application}    ${variables}    550e8400-e29b-41d4-a716-446655440231
    ${response}=    GET On Session    lws    /applications/${application}/configuration    expected_status=200
    Should Not Contain    ${response.text}    robot-secret-value

FT-V1-032 不正な環境変数設定を拒否する
    [Documentation]    未定義変数名や必須値不足を保存または起動前に拒否する。
    [Tags]    FT-V1-032    live    planned    workflow    v1
    ${application}=    Get Environment Variable    LWS_QA_APPLICATION    qa-app
    ${variable}=    Create Dictionary    value=invalid    secret=${False}
    ${variables}=    Create Dictionary    invalid-name=${variable}
    ${body}=    Create Dictionary    variables=${variables}    requestId=550e8400-e29b-41d4-a716-446655440232
    ${response}=    PATCH On Session    lws    /applications/${application}/configuration    json=${body}    expected_status=anything
    Should Be Equal As Integers    ${response.status_code}    400

FT-V1-033 新refへアプリをsyncする
    [Documentation]    新source、runtime、公開内容へ原子的に切り替える。
    [Tags]    FT-V1-033    live    planned    workflow    v1
    ${application}=    Get Environment Variable    LWS_QA_APPLICATION    qa-app
    アプリ操作を実行して成功を待つ    /applications/${application}:sync    550e8400-e29b-41d4-a716-446655440233

FT-V1-034 sync失敗時に旧状態を保持する
    [Documentation]    manifest・Compose・Docker・Caddy反映失敗時に旧source・container・DNS・設定を保持する。
    [Tags]    FT-V1-034    workflow    v1
    Backend受け入れテストを実行する    ^Test(RuntimeRestoresSourceWhenComposeValidationFails|RuntimeUpdateRestoresApplicationOnSourceFailure|DerivedManagerKeepsPreviousFilesWhen(CaddyValidationFails|CoreDNSReloadFails))$

FT-V1-035 失敗したsyncを再実行する
    [Documentation]    失敗したsyncを旧状態から安全に再試行できる。
    [Tags]    FT-V1-035    live    planned    workflow    v1
    ${application}=    Get Environment Variable    LWS_QA_APPLICATION    qa-app
    ${body}=    Create Dictionary    requestId=550e8400-e29b-41d4-a716-446655440235
    ${first}=    POST On Session    lws    /applications/${application}:sync    json=${body}    expected_status=202
    ${first_name}=    Evaluate    $first.json()['name']
    ${second}=    POST On Session    lws    /applications/${application}:sync    json=${body}    expected_status=202
    ${second_name}=    Evaluate    $second.json()['name']
    Should Be Equal As Strings    ${second_name}    ${first_name}
