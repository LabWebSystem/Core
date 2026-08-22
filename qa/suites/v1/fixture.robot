*** Settings ***
Documentation    v1機能テストの前提となるGit fixture基盤
Resource         ../../resources/v1.resource

*** Test Cases ***
TF-V1-001 一時bare Git repositoryを作成する
    [Documentation]    test/apps/validからmain refを持つ一時bare repositoryを作成する。
    [Tags]    TF-V1-001    tooling    v1
    既存fixtureテストを実行して項目を確認する    TF-V1-001

TF-V1-002 GitHub形式URLをcloneする
    [Documentation]    GitHub形式URLからfixtureのcompose.yamlとlws.manifest.yamlをcloneする。
    [Tags]    TF-V1-002    tooling    v1
    既存fixtureテストを実行して項目を確認する    TF-V1-002

TF-V1-003 clone先のrefと内容を確認する
    [Documentation]    clone先のmain refとfixture内容が一致することを確認する。
    [Tags]    TF-V1-003    tooling    v1
    既存fixtureテストを実行して項目を確認する    TF-V1-003

TF-V1-004 作成した一時resourceだけを削除する
    [Documentation]    一時repository、clone先、Git configを削除し、他のresourceを変更しない。
    [Tags]    TF-V1-004    tooling    v1
    既存fixtureテストを実行して項目を確認する    TF-V1-004

TF-V1-005 不正fixture・URL・既存出力先を拒否する
    [Documentation]    不正入力時に作成せず、部分的なresourceを残さず失敗する。
    [Tags]    TF-V1-005    tooling    v1
    既存fixtureテストを実行して項目を確認する    TF-V1-005
