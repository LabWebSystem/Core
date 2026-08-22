*** Settings ***
Documentation    v1のLWSインストール・ライフサイクル受け入れテスト
Resource         ../../resources/v1.resource
Resource         ../../resources/install.resource

*** Test Cases ***
FT-V1-001 Ubuntu系へinstallできる
    [Documentation]    隔離したUbuntuコンテナ内でcurl経由のinstallをrootとして実行する。
    [Tags]    FT-V1-001    lifecycle    v1
    Ubuntuコンテナでinstallスクリプトをroot実行する

FT-V1-002 AlmaLinux系へinstallできる
    [Documentation]    隔離したAlmaLinuxコンテナ内でcurl経由のinstallをrootとして実行する。
    [Tags]    FT-V1-002    lifecycle    v1
    AlmaLinuxコンテナでinstallスクリプトをroot実行する

FT-V1-003 非対応OSでinstallを拒否する
    [Documentation]    隔離した非対応OSコンテナでinstallを拒否する。
    [Tags]    FT-V1-003    lifecycle    v1
    非対応OSコンテナでinstallスクリプトを拒否する

FT-V1-007 起動中LWSをupdateする
    [Documentation]    packageとimageを更新し、必要な場合だけ再起動する。
    [Tags]    FT-V1-007    planned    workflow    v1
    未実装の受け入れテストとして記録する    FT-V1-007

FT-V1-008 停止中LWSの停止状態を維持してupdateする
    [Documentation]    更新後もLWSを停止状態に保つ。
    [Tags]    FT-V1-008    planned    workflow    v1
    未実装の受け入れテストとして記録する    FT-V1-008

FT-V1-009 通常downで保存データを保持する
    [Documentation]    実行環境だけを削除し、設定・状態・永続データを保持する。
    [Tags]    FT-V1-009    planned    workflow    v1
    未実装の受け入れテストとして記録する    FT-V1-009

FT-V1-010 未確認のdown purgeを拒否する
    [Documentation]    down --purgeが確認なしでは削除を実行しない。
    [Tags]    FT-V1-010    planned    workflow    v1
    未実装の受け入れテストとして記録する    FT-V1-010

FT-V1-011 確認済みdown purgeで所有resourceだけを削除する
    [Documentation]    installation IDが一致するLWS所有resourceだけを削除する。
    [Tags]    FT-V1-011    planned    workflow    v1
    未実装の受け入れテストとして記録する    FT-V1-011
