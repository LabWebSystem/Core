*** Settings ***
Library    Process

*** Variables ***
${INSTALL_SCRIPT_URL}    https://raw.githubusercontent.com/LabWebSystem/Core/main/scripts/install.sh

*** Keywords ***
Ubuntuコンテナでinstallスクリプトをroot実行する
    ${script}=    Set Variable    apt-get update && apt-get install -y --no-install-recommends ca-certificates curl sudo && test "$(id -u)" = 0 && curl -fsSL ${INSTALL_SCRIPT_URL} | sudo bash
    ${result}=    Run Process    docker    run    --network    host    --rm    --pull    missing    ubuntu:24.04    bash    -ceu    ${script}    timeout=600s
    Should Be Equal As Integers    ${result.rc}    0    msg=${result.stderr}

AlmaLinuxコンテナでinstallスクリプトをroot実行する
    ${script}=    Set Variable    test "$(id -u)" = 0 && command -v curl >/dev/null && bash /tmp/lws-install.sh
    ${result}=    Run Process    docker    run    --network    host    --rm    --pull    missing    --volume    ${EXECDIR}/scripts/install.sh:/tmp/lws-install.sh:ro    almalinux:9    bash    -ceu    ${script}    timeout=600s
    Should Be Equal As Integers    ${result.rc}    0    msg=${result.stderr}

非対応OSコンテナでinstallスクリプトを拒否する
    ${script}=    Set Variable    apk add --no-cache bash ca-certificates curl sudo && test "$(id -u)" = 0 && ! curl -fsSL ${INSTALL_SCRIPT_URL} | sudo bash
    ${result}=    Run Process    docker    run    --network    host    --rm    --pull    missing    alpine:3.20    sh    -ceu    ${script}    timeout=600s
    Should Be Equal As Integers    ${result.rc}    0    msg=${result.stderr}
