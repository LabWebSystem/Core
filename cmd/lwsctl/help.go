package main

import (
	"fmt"
	"io"
)

func printUsage(w io.Writer) {
	fmt.Fprint(w, `使い方: lwsctl <コマンド> [オプション]

LWSのライフサイクルをDocker Composeで管理します。

コマンド:
  start      LWSを起動します。未設定時はベースドメインを設定します。
  stop       LWS管理下の実行環境を安全かつ冪等に停止します。
  down       LWS管理下の実行環境を停止して削除します。
  status     設定済みのベースドメインと実行環境の状態を表示します。
  rebuild    生成設定を検証し、LWS実行環境を再構成します。
  update     パッケージを更新し、対応するイメージを取得して再起動します。

startのオプション:
  -d, --domain ドメイン  ベースドメインを指定します（例: example.internal）。
                         設定済みの値と異なる場合は、確認後にルーティング設定を再生成します。
  -f, --force              ドメイン変更時の確認を省略します。

downのオプション:
      --purge  設定・状態・永続データも削除します。確認が必要です。
  -f, --force  --purge実行時の確認を省略します。

ヘルプ:
  -h, --help  このヘルプを表示します。コマンドの後ろに指定すると、そのコマンドの詳細を表示します。

使用例:
  sudo lwsctl start --domain example.internal
  sudo lwsctl start -d new.internal --force
  sudo lwsctl status
  sudo lwsctl down --purge --force
`)
}

func printCommandUsage(w io.Writer, command string) {
	switch command {
	case "start":
		fmt.Fprint(w, `使い方: lwsctl start [--domain ドメイン] [--force]

LWSを起動します。設定がない場合はベースドメインを入力して初期設定を作成します。
設定済みのドメインを変更すると、DNSとReverse Proxyの設定を再生成します。

オプション:
  -d, --domain ドメイン  ベースドメインを指定します（例: example.internal）。
  -f, --force              設定済みドメインを変更する際の確認を省略します。
  -h, --help               このヘルプを表示します。
`)
	case "stop":
		fmt.Fprint(w, "使い方: lwsctl stop\n\nLWS管理下のDocker Compose実行環境を安全かつ冪等に停止します。\n")
	case "down":
		fmt.Fprint(w, `使い方: lwsctl down [--purge] [--force]

LWS実行環境を停止して削除します。通常は設定と永続データを保持します。パッケージの削除はAPT/DNFで行ってください。

オプション:
      --purge  設定・状態・永続データも削除します。確認が必要です。
  -f, --force  --purge実行時の確認を省略します。
  -h, --help   このヘルプを表示します。
`)
	case "status":
		fmt.Fprint(w, "使い方: lwsctl status\n\n設定済みのベースドメインとDocker Compose実行環境の状態を表示します。状態は変更しません。\n")
	case "rebuild":
		fmt.Fprint(w, "使い方: lwsctl rebuild\n\n設定を検証し、LWS実行環境と生成設定を再構成します。パッケージは再インストールしません。\n")
	case "update":
		fmt.Fprint(w, "使い方: lwsctl update\n\nパッケージ更新をAPT/DNFへ委譲し、対応するDockerイメージを取得してLWSを再起動します。事前にstartで設定を作成してください。\n")
	default:
		printUsage(w)
	}
}
