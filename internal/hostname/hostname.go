package hostname

import "strings"

// Publicはアプリ名とベースドメインから単一ラベルの公開ホスト名を生成する。
func Public(appName, baseDomain string) string {
	lastDot := strings.LastIndexByte(baseDomain, '.')
	if lastDot < 0 {
		return appName + "-" + baseDomain
	}
	prefix := strings.ReplaceAll(baseDomain[:lastDot], ".", "-")
	return appName + "-" + prefix + baseDomain[lastDot:]
}
