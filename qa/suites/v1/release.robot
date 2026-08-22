*** Settings ***
Documentation    v1の配布・リリース受け入れテスト
Resource         ../../resources/v1.resource

*** Test Cases ***
FT-V1-070 Core packageとchecksumを生成する
    [Documentation]    Core versionを設定して.deb、.rpm、checksumを生成する。
    [Tags]    FT-V1-070    planned    package    v1
    未実装の受け入れテストとして記録する    FT-V1-070

FT-V1-071 package更新時に既存設定を安全に移行する
    [Documentation]    既存configの権限を安全に移行し既存データを壊さない。
    [Tags]    FT-V1-071    planned    lifecycle    v1
    未実装の受け入れテストとして記録する    FT-V1-071

FT-V1-072 LWS releaseのversionを一致させる
    [Documentation]    package、lwsctl、Compose、Backend imageのversionが一致する。
    [Tags]    FT-V1-072    planned    release    v1
    未実装の受け入れテストとして記録する    FT-V1-072

FT-V1-073 OpenAPI生成物の差分を検出する
    [Documentation]    OpenAPI生成物に差分がある場合にcheck-openapiまたはCIが失敗する。
    [Tags]    FT-V1-073    planned    contract    v1
    未実装の受け入れテストとして記録する    FT-V1-073
