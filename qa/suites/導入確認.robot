*** Settings ***
Documentation    LWS機能受け入れテスト基盤の導入確認
Library          Process

*** Test Cases ***
QA-V1-001 Robot Frameworkでテストスイートを実行できる
    [Documentation]    Robot Frameworkが受け入れテストを実行し、結果を記録できることを確認する。
    [Tags]    tooling    v1
    ${result}=    Run Process    python3    -c    print('Robot Frameworkの実行に成功しました')
    Should Be Equal As Integers    ${result.rc}    0
    Should Contain    ${result.stdout}    Robot Frameworkの実行に成功しました
