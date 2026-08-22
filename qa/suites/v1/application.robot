*** Settings ***
Documentation    v1のアプリ登録・公開・設定・sync受け入れテスト
Resource         ../../resources/v1.resource

*** Test Cases ***
FT-V1-020 適合GitHub repositoryを登録して公開する
    [Documentation]    Operation完了後にsource取得・検証・起動しsubdomain.domainで公開する。
    [Tags]    FT-V1-020    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-020    適合GitHub repositoryを登録    ACTIVEになりDNSとHTTPで公開する

FT-V1-021 不正なrepository URLを拒否する
    [Documentation]    GitHub以外、SSH、認証情報付きURLをGit実行前に拒否する。
    [Tags]    FT-V1-021    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-021    不正repository URLを登録    Gitを実行せず拒否する

FT-V1-022 manifest指定のserviceとportで公開する
    [Documentation]    正しいmanifestとComposeを登録し指定service・portで公開する。
    [Tags]    FT-V1-022    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-022    正しいmanifestとComposeを登録    指定service・portで公開する

FT-V1-023 不正manifestをDocker実行前に拒否する
    [Documentation]    manifest不在・破損・schema違反でDockerを実行せず既存状態を保持する。
    [Tags]    FT-V1-023    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-023    不正manifestを登録    Dockerを実行せず既存状態を保持する

FT-V1-024 危険なsource記法を拒否する
    [Documentation]    symlink、anchor、duplicate key、外部読込を含むsourceを採用しない。
    [Tags]    FT-V1-024    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-024    危険なsourceを登録    sourceを採用せず拒否する

FT-V1-025 危険なComposeを起動前に拒否する
    [Documentation]    root外path、bind mount、host port、privileged、Docker socketを拒否する。
    [Tags]    FT-V1-025    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-025    危険なComposeを登録    Compose起動前に拒否する

FT-V1-026 Git取得失敗時に旧公開経路を保持する
    [Documentation]    cloneまたはref取得失敗時にOperationをfailedとし旧source・公開経路を維持する。
    [Tags]    FT-V1-026    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-026    Git cloneまたはref取得を失敗させる    旧sourceと旧公開経路を保持する

FT-V1-027 アプリ間の公開とnetworkを分離する
    [Documentation]    同じCompose service名のアプリを登録しても各URLが正しく到達し相互通信を拒否する。
    [Tags]    FT-V1-027    planned    integration    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-027    同じservice名のアプリを2つ登録    URLを分離しアプリ間通信を拒否する

FT-V1-028 アプリのstop/start/rebuildでvolumeを保持する
    [Documentation]    起動中アプリを操作し対象だけを変更してnamed volumeを保持する。
    [Tags]    FT-V1-028    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-028    起動中アプリをstop/start/rebuild    対象だけを操作しvolumeを保持する

FT-V1-030 必須環境変数を登録して起動する
    [Documentation]    起動直前にapp.envを生成しアプリを起動する。
    [Tags]    FT-V1-030    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-030    必須環境変数を登録してstart    app.envを生成し起動する

FT-V1-031 secretを外部へ漏えいさせない
    [Documentation]    secret値が取得結果、API、Operation、ログ、エラーへ含まれない。
    [Tags]    FT-V1-031    planned    contract    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-031    secretを取得・ログ・Operation・エラーで確認    secret値を返さない

FT-V1-032 不正な環境変数設定を拒否する
    [Documentation]    未定義変数名や必須値不足を保存または起動前に拒否する。
    [Tags]    FT-V1-032    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-032    不正な環境変数を登録・起動    保存または起動を拒否する

FT-V1-033 新refへアプリをsyncする
    [Documentation]    新source、runtime、公開内容へ原子的に切り替える。
    [Tags]    FT-V1-033    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-033    新refへsync    新sourceと公開内容へ原子的に切り替える

FT-V1-034 sync失敗時に旧状態を保持する
    [Documentation]    manifest・Compose・Docker・Caddy反映失敗時に旧source・container・DNS・設定を保持する。
    [Tags]    FT-V1-034    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-034    sync途中で反映を失敗させる    旧状態を保持する

FT-V1-035 失敗したsyncを再実行する
    [Documentation]    失敗したsyncを旧状態から安全に再試行できる。
    [Tags]    FT-V1-035    planned    workflow    v1
    未実装の受け入れシナリオを失敗させる    FT-V1-035    失敗したsyncを再実行    旧状態から再試行できる
