*** Settings ***
Library    OperatingSystem
Library    Process

*** Variables ***
${WORKSPACE}    ${CURDIR}/../..
${QA_ROOT}      ${EMPTY}
${LWSCTL}       ${EMPTY}
${COMPOSE_FILE}    ${EMPTY}
${BACKEND_IMAGE}    ${EMPTY}
${AVAHI_IMAGE}    ${EMPTY}

*** Keywords ***
LWS本体の隔離環境を準備する
    ${qa_root}=    Evaluate    __import__('tempfile').mkdtemp(prefix='lws-qa-lifecycle-')
    Set Suite Variable    ${QA_ROOT}    ${qa_root}
    ${project}=    Evaluate    'lws-qa-' + __import__('uuid').uuid4().hex[:8]
    Set Suite Variable    ${QA_PROJECT}    ${project}
    ${backend_image}=    Set Variable    ${project}-backend:local
    ${avahi_image}=    Set Variable    ${project}-avahi:local
    Set Suite Variable    ${BACKEND_IMAGE}    ${backend_image}
    Set Suite Variable    ${AVAHI_IMAGE}    ${avahi_image}
    ${compose_file}=    Set Variable    ${qa_root}/compose.json
    Set Suite Variable    ${COMPOSE_FILE}    ${compose_file}
    Set Environment Variable    LWS_CONFIG_DIR    ${qa_root}/etc
    Set Environment Variable    LWS_STATE_DIR    ${qa_root}/state
    Set Environment Variable    LWS_VERSION_FILE    ${qa_root}/version
    Set Environment Variable    LWS_COMPOSE_FILE    ${compose_file}
    Set Environment Variable    LWS_COMPOSE_PROJECT    ${project}
    Set Environment Variable    LWS_CADDY_CONTAINER    ${project}-caddy-1
    Set Environment Variable    LWS_AVAHI_CONTAINER    ${project}-avahi-1
    Create File    ${qa_root}/version    0.1.0
    ${build}=    Run Process    ${WORKSPACE}/scripts/build-lwsctl.sh    --output    ${qa_root}/lwsctl    cwd=${WORKSPACE}
    Should Be Equal As Integers    ${build.rc}    0
    Set Suite Variable    ${LWSCTL}    ${qa_root}/lwsctl
    ${compose_prepare}=    Run Process    python3    ${WORKSPACE}/scripts/prepare-lifecycle-compose.py    --output    ${compose_file}    --project    ${project}    --backend-image    ${backend_image}    --avahi-image    ${avahi_image}    cwd=${WORKSPACE}
    Should Be Equal As Integers    ${compose_prepare.rc}    0    ${compose_prepare.stderr}
    ${ports}=    Run Process    python3    -c    import re,subprocess,sys; sys.exit(1 if re.search(r':(?:80|5353)(?:\\s|$)',subprocess.check_output(['ss','-H','-ltnup']).decode()) else 0)
    Run Keyword If    ${ports.rc} != 0    Skip    LWSの必須ポート(80/tcp、5353/udp)が既に使用されています。
    ${avahi_build}=    Run Process    docker    build    --network    host    --file    ${WORKSPACE}/infrastructure/Dockerfile.avahi    --tag    ${AVAHI_IMAGE}    ${WORKSPACE}
    Should Be Equal As Integers    ${avahi_build.rc}    0

LWS本体を停止して隔離環境を削除する
    Run Keyword If    $QA_ROOT and $LWSCTL    Run Process    ${LWSCTL}    down    --recursive    --purge    --force
    Run Keyword If    $COMPOSE_FILE    Run Process    docker    compose    --project-name    ${QA_PROJECT}    --file    ${COMPOSE_FILE}    down    --volumes    --remove-orphans
    Run Keyword If    $BACKEND_IMAGE    Run Process    docker    image    rm    ${BACKEND_IMAGE}    ${AVAHI_IMAGE}
    Run Keyword If    $QA_ROOT    Remove Directory    ${QA_ROOT}    recursive=True

LWSを起動する
    ${result}=    Run Process    ${LWSCTL}    start    --domain    qa.example.local
    Log    stdout=${result.stdout} stderr=${result.stderr}
    Should Be Equal As Integers    ${result.rc}    0

LWSが起動済みであることを確認する
    ${status}=    Run Process    ${LWSCTL}    status
    Should Be Equal As Integers    ${status.rc}    0
    Should Contain    ${status.stdout}    設定済み: YES
    ${services}=    Run Process    docker    compose    --project-name    ${QA_PROJECT}    --file    ${COMPOSE_FILE}    ps    --status    running    --services
    Should Be Equal As Integers    ${services.rc}    0
    Should Contain    ${services.stdout}    backend
