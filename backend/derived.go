package backend

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/LabWebSystem/Core/internal/hostname"
)

type PublishedApplication struct {
	Subdomain string
	AppID     string
	Service   string
	Port      int
	IsSystem  bool
}

func GenerateHosts(baseDomain, publicAddress string, apps []PublishedApplication) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", publicAddress, hostname.Public("api", baseDomain))
	for _, a := range apps {
		fmt.Fprintf(&b, "%s %s\n", publicAddress, hostname.Public(a.Subdomain, baseDomain))
	}
	return b.String()
}
func GenerateCaddyfile(baseDomain string, apps []PublishedApplication) string {
	var b strings.Builder
	apiHost := hostname.Public("api", baseDomain)
	fmt.Fprintf(&b, "{\n\tauto_https off\n}\n\nhttp://%s {\n\treverse_proxy backend:8080\n}\n\n", apiHost)
	for _, a := range apps {
		if a.IsSystem {
			fmt.Fprintf(&b, "http://%s {\n\t@backend path /api/* /openapi.json /swagger /swagger/*\n\thandle @backend {\n\t\treverse_proxy backend:8080 {\n\t\t\theader_up Host %s\n\t\t}\n\t}\n\treverse_proxy lws-%s:%d\n}\n", hostname.Public(a.Subdomain, baseDomain), apiHost, a.AppID, a.Port)
			continue
		}
		fmt.Fprintf(&b, "http://%s {\n\treverse_proxy lws-%s:%d\n}\n", hostname.Public(a.Subdomain, baseDomain), a.AppID, a.Port)
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
