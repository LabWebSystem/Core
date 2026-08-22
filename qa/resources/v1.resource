*** Settings ***
Library    Process

*** Variables ***
${ROOT}    ${CURDIR}/../..

*** Keywords ***
未実装の受け入れシナリオを失敗させる
    [Arguments]    ${id}    ${operation}    ${expected}
    Fail    ${id}: 未実装です。操作=${operation} / 期待結果=${expected}

既存fixtureテストを実行して項目を確認する
    [Arguments]    ${id}
    ${result}=    Run Process    ${ROOT}/scripts/test-fixtures.sh    cwd=${ROOT}
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    TEST_ITEM|${id}|成功|
