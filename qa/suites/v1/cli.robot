*** Settings ***
Documentation    v1のLWSインストール・ライフサイクル受け入れテスト
Resource         ../../resources/v1.resource

*** Test Cases ***
FT-V1-001 Ubuntu系へinstallできる
    [Documentation]    .debを配置し、インストール時にLWSを自動起動しない。
    [Tags]    FT-V1-001    planned    lifecycle    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-001    Ubuntu系へinstall    .debが配置されLWSは自動起動しない

FT-V1-002 AlmaLinux系へinstallできる
    [Documentation]    .rpmを配置し、インストール時にLWSを自動起動しない。
    [Tags]    FT-V1-002    planned    lifecycle    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-002    AlmaLinux系へinstall    .rpmが配置されLWSは自動起動しない

FT-V1-003 非対応OSでinstallを拒否する
    [Documentation]    環境を変更せず日本語で失敗する。
    [Tags]    FT-V1-003    planned    lifecycle    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-003    非対応OSでinstall    環境を変更せず日本語で失敗する

FT-V1-004 初回startでLWSを起動する
    [Documentation]    設定、secret、installation ID、公開IP、Composeを準備して起動する。
    [Tags]    FT-V1-004    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-004    初回start --domain    設定とComposeが準備され起動する

FT-V1-005 不正domainでstartを拒否する
    [Documentation]    設定を変更せず、Composeを起動しない。
    [Tags]    FT-V1-005    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-005    不正domainでstart    設定と実行環境を変更しない

FT-V1-006 start済みLWSを安全に操作する
    [Documentation]    stop、status、rebuildが対象だけを操作し設定・状態・volumeを保持する。
    [Tags]    FT-V1-006    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-006    start済みLWSをstop/status/rebuild    対象だけを操作し保存データを保持する

FT-V1-007 起動中LWSをupdateする
    [Documentation]    packageとimageを更新し、必要な場合だけ再起動する。
    [Tags]    FT-V1-007    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-007    起動中LWSをupdate    更新後に必要な場合だけ再起動する

FT-V1-008 停止中LWSの停止状態を維持してupdateする
    [Documentation]    更新後もLWSを停止状態に保つ。
    [Tags]    FT-V1-008    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-008    停止中LWSをupdate    更新後も停止状態を維持する

FT-V1-009 通常downで保存データを保持する
    [Documentation]    実行環境だけを削除し、設定・状態・永続データを保持する。
    [Tags]    FT-V1-009    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-009    通常down    設定・状態・永続データを保持する

FT-V1-010 未確認のdown purgeを拒否する
    [Documentation]    down --purgeが確認なしでは削除を実行しない。
    [Tags]    FT-V1-010    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-010    down --purgeを未確認で実行    確認を要求し削除しない

FT-V1-011 確認済みdown purgeで所有resourceだけを削除する
    [Documentation]    installation IDが一致するLWS所有resourceだけを削除する。
    [Tags]    FT-V1-011    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-011    down --purge --force    LWS所有resourceだけを削除する
