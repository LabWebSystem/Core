*** Settings ***
Library    OperatingSystem
Library    Process
Library    String

*** Variables ***
${WORKSPACE}    ${CURDIR}/../..
${QA_ROOT}      ${EMPTY}
${LWSCTL}       ${EMPTY}

*** Keywords ***
LWS本体の隔離環境を準備する
    ${qa_root}=    Evaluate    __import__('tempfile').mkdtemp(prefix='lws-qa-lifecycle-')
    Set Suite Variable    ${QA_ROOT}    ${qa_root}
    ${project}=    Evaluate    'lws-qa-' + __import__('uuid').uuid4().hex[:8]
    Set Suite Variable    ${QA_PROJECT}    ${project}
    Create File    ${qa_root}/version    0.1.0
    ${build}=    Run Process    ${WORKSPACE}/scripts/build-lwsctl.sh    --output    ${qa_root}/lwsctl    cwd=${WORKSPACE}
    Should Be Equal As Integers    ${build.rc}    0    lwsctl buildに失敗: stdout=${build.stdout} stderr=${build.stderr}
    Set Suite Variable    ${LWSCTL}    ${qa_root}/lwsctl
    ${compose}=    Get File    ${WORKSPACE}/infrastructure/compose.yaml
    ${compose}=    Replace String    ${compose}    lws-backend-data    ${project}-backend-data
    ${compose}=    Replace String    ${compose}    lws-backend-config    ${project}-backend-config
    ${compose}=    Replace String    ${compose}    lws-generated    ${project}-generated
    ${compose}=    Replace String    ${compose}    lws-internal    ${project}-internal
    ${compose}=    Replace String    ${compose}    context: ..    context: ${WORKSPACE}
    ${compose}=    Evaluate    __import__('re').sub(r'(?m)^\\s*image: ghcr[.]io/.*lws-backend.*\\n', '', $compose)
    ${compose}=    Evaluate    __import__('re').sub(r'(?m)^ *image: ghcr[.]io/.*lws-avahi:.*$', lambda line: (' ' * 4) + 'image: lws-avahi-qa:local', $compose)
    Should Contain    ${compose}    image: lws-avahi-qa:local
    ${compose_file}=    Set Variable    ${qa_root}/compose.yaml
    Create File    ${compose_file}    ${compose}
    Set Environment Variable    LWS_CONFIG_DIR    ${qa_root}/etc
    Set Environment Variable    LWS_STATE_DIR    ${qa_root}/state
    Set Environment Variable    LWS_VERSION_FILE    ${qa_root}/version
    Set Environment Variable    LWS_COMPOSE_FILE    ${compose_file}
    Set Environment Variable    LWS_COMPOSE_PROJECT    ${project}
    Set Environment Variable    LWS_CADDY_CONTAINER    ${project}-caddy-1
    Set Environment Variable    LWS_AVAHI_CONTAINER    ${project}-avahi-1
    ${ports}=    Run Process    python3    -c    import re,subprocess,sys; sys.exit(1 if re.search(r':(?:80|5353)(?:\\s|$)',subprocess.check_output(['ss','-H','-ltnup']).decode()) else 0)
    Run Keyword If    ${ports.rc} != 0    Skip    LWSの必須ポート(80/tcp、5353/udp)が既に使用されています。
    ${avahi_build}=    Run Process    docker    build    --network    host    --file    ${WORKSPACE}/infrastructure/Dockerfile.avahi    --tag    lws-avahi-qa:local    ${WORKSPACE}
    Should Be Equal As Integers    ${avahi_build.rc}    0    Avahi image buildに失敗: stdout=${avahi_build.stdout} stderr=${avahi_build.stderr}

LWS本体を停止して隔離環境を削除する
    Run Keyword If    $QA_ROOT    Run Process    ${LWSCTL}    down    --purge    --force    --recursive
    Run Keyword If    $QA_ROOT    Remove Directory    ${QA_ROOT}    recursive=True

LWSを起動する
    ${result}=    Run Process    ${LWSCTL}    start    --domain    qa.example.local
    Log    stdout=${result.stdout} stderr=${result.stderr}
    Should Be Equal As Integers    ${result.rc}    0    lwsctl startに失敗: stdout=${result.stdout} stderr=${result.stderr}

LWSが起動済みであることを確認する
    ${status}=    Run Process    ${LWSCTL}    status
    Should Be Equal As Integers    ${status.rc}    0
    Should Contain    ${status.stdout}    設定済み: YES
    ${services}=    Run Process    docker    compose    --project-name    ${QA_PROJECT}    --file    ${WORKSPACE}/infrastructure/compose.yaml    ps    --status    running    --services
    Should Be Equal As Integers    ${services.rc}    0    Docker Compose psに失敗: stdout=${services.stdout} stderr=${services.stderr}
    Should Contain    ${services.stdout}    backend
