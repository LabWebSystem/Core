*** Settings ***
Library    Process

*** Variables ***
${ROOT}    ${CURDIR}/../..

*** Keywords ***
Backend受け入れテストを実行する
    [Arguments]    ${pattern}
    ${result}=    Run Process    go    test    ./backend/...    -run    ${pattern}    cwd=${ROOT}
    Log    stdout=${result.stdout} stderr=${result.stderr}
    Should Be Equal As Integers    ${result.rc}    0

CLI受け入れテストを実行する
    [Arguments]    ${target}=cli
    ${result}=    Run Process    ${ROOT}/scripts/test.sh    ${target}    cwd=${ROOT}
    Log    stdout=${result.stdout} stderr=${result.stderr}
    Should Be Equal As Integers    ${result.rc}    0

OpenAPI受け入れテストを実行する
    ${result}=    Run Process    mise    run    check-openapi    cwd=${ROOT}
    Log    stdout=${result.stdout} stderr=${result.stderr}
    Should Be Equal As Integers    ${result.rc}    0

既存fixtureテストを実行して項目を確認する
    [Arguments]    ${id}
    ${result}=    Run Process    ${ROOT}/scripts/test-fixtures.sh    cwd=${ROOT}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    TEST_ITEM|${id}|成功|
