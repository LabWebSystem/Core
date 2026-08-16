package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type PublishedApplication struct {
	Subdomain string
	AppID     string
	Service   string
	Port      int
}

func GenerateHosts(baseDomain, publicAddress string, apps []PublishedApplication) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s api.%s\n", publicAddress, baseDomain)
	for _, a := range apps {
		fmt.Fprintf(&b, "%s %s.%s\n", publicAddress, a.Subdomain, baseDomain)
	}
	return b.String()
}
func GenerateCaddyfile(baseDomain string, apps []PublishedApplication) string {
	var b strings.Builder
	fmt.Fprintf(&b, "{\n\tauto_https off\n}\n\nhttp://api.%s {\n\treverse_proxy backend:8080\n}\n", baseDomain)
	for _, a := range apps {
		fmt.Fprintf(&b, "http://%s.%s {\n\treverse_proxy lws-%s:%d\n}\n", a.Subdomain, baseDomain, a.AppID, a.Port)
	}
	return b.String()
}
func WriteAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".lws-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
