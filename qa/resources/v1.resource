*** Settings ***
Library    Process

*** Variables ***
${ROOT}    ${CURDIR}/../..

*** Keywords ***
未実装の受け入れテストとして記録する
    [Arguments]    ${id}
    Fail    ${id}の実環境を用いる受け入れテストは未実装です。
