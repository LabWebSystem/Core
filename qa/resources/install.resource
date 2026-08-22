*** Settings ***
Library    Process

*** Variables ***
${INSTALL_SCRIPT_URL}    https://raw.githubusercontent.com/LabWebSystem/Core/main/scripts/install.sh

*** Keywords ***
Ubuntuコンテナでinstallスクリプトをroot実行する
    ${script}=    Set Variable    apt-get update && apt-get install -y --no-install-recommends ca-certificates curl sudo && test "$(id -u)" = 0 && curl -fsSL ${INSTALL_SCRIPT_URL} | sudo bash
    ${result}=    Run Process    docker    run    --rm    --pull    missing    ubuntu:24.04    bash    -ceu    ${script}    timeout=600s
    Should Be Equal As Integers    ${result.rc}    0    msg=${result.stderr}

AlmaLinuxコンテナでinstallスクリプトをroot実行する
    ${script}=    Set Variable    dnf install -y ca-certificates curl-minimal sudo && test "$(id -u)" = 0 && curl -fsSL ${INSTALL_SCRIPT_URL} | sudo bash
    ${result}=    Run Process    docker    run    --rm    --pull    missing    almalinux:9    bash    -ceu    ${script}    timeout=600s
    Should Be Equal As Integers    ${result.rc}    0    msg=${result.stderr}

非対応OSコンテナでinstallスクリプトを拒否する
    ${script}=    Set Variable    apk add --no-cache bash ca-certificates curl sudo && test "$(id -u)" = 0 && ! curl -fsSL ${INSTALL_SCRIPT_URL} | sudo bash
    ${result}=    Run Process    docker    run    --rm    --pull    missing    alpine:3.20    sh    -ceu    ${script}    timeout=600s
    Should Be Equal As Integers    ${result.rc}    0    msg=${result.stderr}
