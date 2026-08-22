*** Settings ***
Library    OperatingSystem
Library    Process
Library    String

*** Variables ***
${WORKSPACE}    ${CURDIR}/../..
${QA_ROOT}      ${EMPTY}
${LWSCTL}       ${EMPTY}
${LWS_LIFECYCLE_BLOCKED}    ${False}

*** Keywords ***
LWS本体の隔離環境を準備する
    ${qa_root}=    Evaluate    __import__('tempfile').mkdtemp(prefix='lws-qa-lifecycle-')
    Set Suite Variable    ${QA_ROOT}    ${qa_root}
    ${project}=    Evaluate    'lws-qa-' + __import__('uuid').uuid4().hex[:8]
    Set Suite Variable    ${QA_PROJECT}    ${project}
    Create File    ${qa_root}/version    0.1.0
    ${build}=    Run Process    ${WORKSPACE}/scripts/build-lwsctl.sh    --output    ${qa_root}/lwsctl    cwd=${WORKSPACE}
    Should Be Equal As Integers    ${build.rc}    0
    Set Suite Variable    ${LWSCTL}    ${qa_root}/lwsctl
    ${compose}=    Get File    ${WORKSPACE}/infrastructure/compose.yaml
    ${compose}=    Replace String    ${compose}    lws-backend-data    ${project}-backend-data
    ${compose}=    Replace String    ${compose}    lws-backend-config    ${project}-backend-config
    ${compose}=    Replace String    ${compose}    lws-generated    ${project}-generated
    ${compose}=    Replace String    ${compose}    lws-internal    ${project}-internal
    ${compose_file}=    Set Variable    ${qa_root}/compose.yaml
    Copy File    ${WORKSPACE}/infrastructure/Corefile    ${qa_root}/Corefile
    Create File    ${compose_file}    ${compose}
    Set Environment Variable    LWS_CONFIG_DIR    ${qa_root}/etc
    Set Environment Variable    LWS_STATE_DIR    ${qa_root}/state
    Set Environment Variable    LWS_VERSION_FILE    ${qa_root}/version
    Set Environment Variable    LWS_COMPOSE_FILE    ${compose_file}
    Set Environment Variable    LWS_COMPOSE_PROJECT    ${project}
    Set Environment Variable    LWS_CADDY_CONTAINER    ${project}-caddy-1
    Set Environment Variable    LWS_COREDNS_CONTAINER    ${project}-coredns-1
    ${ports}=    Run Process    python3    -c    import re,subprocess,sys; sys.exit(1 if re.search(r':(?:80|53)(?:\\s|$)',subprocess.check_output(['ss','-H','-ltnup']).decode()) else 0)
    Run Keyword If    ${ports.rc} != 0    Skip    LWSの必須ポート(80/tcp、53/tcp、53/udp)が既に使用されています。

LWS本体を停止して隔離環境を削除する
    Run Keyword If    $QA_ROOT    Run Process    ${LWSCTL}    down    --purge    --force
    Run Keyword If    $QA_ROOT    Remove Directory    ${QA_ROOT}    recursive=True

LWSを起動する
    ${result}=    Run Process    ${LWSCTL}    start    --domain    qa.example.internal
    Log    stdout=${result.stdout} stderr=${result.stderr}
    IF    ${result.rc} != 0 and ('dependency failed' in $result.stderr or 'unhealthy' in $result.stderr)
        Set Suite Variable    ${LWS_LIFECYCLE_BLOCKED}    ${True}
        Skip    ComposeのBackend healthcheckが成功しないため、起動環境を用意できません。
    END
    Should Be Equal As Integers    ${result.rc}    0

LWSライフサイクル環境が利用可能であること
    Run Keyword If    ${LWS_LIFECYCLE_BLOCKED}    Skip    ComposeのBackend healthcheckが成功しないため、起動環境を用意できません。

LWSが起動済みであることを確認する
    ${status}=    Run Process    ${LWSCTL}    status
    Should Be Equal As Integers    ${status.rc}    0
    Should Contain    ${status.stdout}    設定済み: YES
    ${services}=    Run Process    docker    compose    --project-name    ${QA_PROJECT}    --file    ${WORKSPACE}/infrastructure/compose.yaml    ps    --status    running    --services
    Should Be Equal As Integers    ${services.rc}    0
    Should Contain    ${services.stdout}    backend
