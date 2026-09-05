package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type ComposeSource struct {
	Path string
	Data []byte
}

// ReadComposeSourcesはメインComposeと、指定順のoverrideを同じ順序で読み取る。
func ReadComposeSources(sourceRoot, composeFile string, overrideFiles []string) ([]ComposeSource, error) {
	files := append([]string{composeFile}, overrideFiles...)
	result := make([]ComposeSource, 0, len(files))
	for _, file := range files {
		if !containsComposeFile(file) {
			return nil, NewValidationError("composeFile", "Composeファイル名が不正です", "INVALID_ARGUMENT")
		}
		path := filepath.Join(sourceRoot, file)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("Composeを読み取れません: %w", err)
		}
		result = append(result, ComposeSource{Path: path, Data: data})
	}
	return result, nil
}

func MergeComposeVariables(sources []ComposeSource) ([]ComposeVariable, error) {
	byName := map[string]ComposeVariable{}
	for _, source := range sources {
		variables, err := ExtractComposeVariables(source.Data)
		if err != nil {
			return nil, err
		}
		for _, variable := range variables {
			byName[variable.Name] = variable
		}
	}
	result := make([]ComposeVariable, 0, len(byName))
	for _, variable := range byName {
		result = append(result, variable)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

type WebInterface struct {
	Service string
	Port    int
}

func ComposeHasService(sources []ComposeSource, name string) bool {
	for _, source := range sources {
		var model struct {
			Services map[string]any `yaml:"services"`
		}
		if yaml.Unmarshal(source.Data, &model) == nil {
			if _, ok := model.Services[name]; ok {
				return true
			}
		}
	}
	return false
}

func ComposeWebInterfaces(sources []ComposeSource) ([]WebInterface, error) {
	seen := map[string]bool{}
	result := []WebInterface{}
	for _, source := range sources {
		var model struct {
			Services map[string]struct {
				Ports  []any `yaml:"ports"`
				Expose []any `yaml:"expose"`
			} `yaml:"services"`
		}
		if err := yaml.Unmarshal(source.Data, &model); err != nil {
			return nil, NewValidationError("compose", "Compose YAMLが不正です", "INVALID_COMPOSE")
		}
		for service, definition := range model.Services {
			portValues := append(append([]any{}, definition.Ports...), definition.Expose...)
			for _, raw := range portValues {
				port, ok := composeTargetPort(raw)
				if !ok || port < 1 || port > 65535 {
					continue
				}
				key := fmt.Sprintf("%s:%d", service, port)
				if !seen[key] {
					seen[key] = true
					result = append(result, WebInterface{Service: service, Port: port})
				}
			}
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Service == result[j].Service {
			return result[i].Port < result[j].Port
		}
		return result[i].Service < result[j].Service
	})
	return result, nil
}

func composeTargetPort(raw any) (int, bool) {
	switch value := raw.(type) {
	case string:
		value = strings.SplitN(value, "/", 2)[0]
		parts := strings.Split(value, ":")
		value = parts[len(parts)-1]
		port, err := strconv.Atoi(value)
		return port, err == nil
	case map[string]any:
		target, ok := value["target"]
		if !ok {
			return 0, false
		}
		switch port := target.(type) {
		case int:
			return port, true
		case int64:
			return int(port), true
		case uint64:
			return int(port), true
		case float64:
			return int(port), port == float64(int(port))
		}
	}
	return 0, false
}

var forbiddenComposeKeys = map[string]string{"include": "外部Composeのincludeは許可されていません", "extends": "Composeのextendsは許可されていません", "env_file": "env_fileは許可されていません", "label_file": "label_fileは許可されていません", "volumes_from": "volumes_fromは許可されていません", "privileged": "privilegedは許可されていません", "network_mode": "host networkは許可されていません", "pid": "host PIDは許可されていません", "ipc": "host IPCは許可されていません", "tmpfs": "tmpfsは許可されていません", "configs": "ファイル型configsは許可されていません", "secrets": "ファイル型secretsは許可されていません", "additional_contexts": "追加build contextは許可されていません"}

func ValidateComposeSource(root string, data []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return NewValidationError("compose", "Compose YAMLが不正です", "INVALID_COMPOSE")
	}
	if err := walkCompose(node.Content, root); err != nil {
		return err
	}
	if err := rejectSourceBindMounts(data); err != nil {
		return err
	}
	if err := validateComposeWatchPaths(root, data); err != nil {
		return err
	}
	if _, err := ComposeDeviceAttachments(data); err != nil {
		return err
	}
	return nil
}

// rejectSourceBindMountsはDocker daemonが読むhost pathへの依存を、正規化や
// 自動置換より前に拒否する。アプリの永続領域はLWS所有Named Volumeだけを使う。
func rejectSourceBindMounts(data []byte) error {
	var model struct {
		Services map[string]struct {
			Volumes []any `yaml:"volumes"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &model); err != nil {
		return NewValidationError("compose", "Compose YAMLが不正です", "INVALID_COMPOSE")
	}
	for service, definition := range model.Services {
		for _, volume := range definition.Volumes {
			if isBindMount(volume) {
				return NewValidationError("services."+service+".volumes", "host bind mountまたは匿名volumeは許可されていません。LWS所有のNamed Volumeを使用してください", "BIND_MOUNT_FORBIDDEN")
			}
		}
	}
	return nil
}

// validateComposeWatchPathsは、開発時の同期元をアプリsource内に限定する。
// Watchはhost bind mountの代替であり、ホスト固有のpathを受け入れる仕組みではない。
func validateComposeWatchPaths(root string, data []byte) error {
	var model struct {
		Services map[string]struct {
			Develop struct {
				Watch []struct {
					Path string `yaml:"path"`
				} `yaml:"watch"`
			} `yaml:"develop"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &model); err != nil {
		return NewValidationError("compose", "Compose YAMLが不正です", "INVALID_COMPOSE")
	}
	for service, definition := range model.Services {
		for _, watch := range definition.Develop.Watch {
			if watch.Path == "" {
				return NewValidationError("services."+service+".develop.watch.path", "Compose Watchのpathを指定してください", "INVALID_COMPOSE")
			}
			if err := ValidateProjectPath(root, watch.Path); err != nil {
				return NewValidationError("services."+service+".develop.watch.path", "Compose Watchのpathはプロジェクトrootからの相対pathで指定してください", "PATH_OUTSIDE_PROJECT_ROOT")
			}
		}
	}
	return nil
}

func walkCompose(nodes []*yaml.Node, root string) error {
	for _, n := range nodes {
		if n.Kind == yaml.AliasNode || n.Anchor != "" || n.Style&yaml.TaggedStyle != 0 {
			return NewValidationError("compose", "Composeにaliasまたは独自tagは許可されていません", "INVALID_COMPOSE")
		}
		if n.Kind == yaml.MappingNode {
			seen := map[string]bool{}
			for i := 0; i < len(n.Content); i += 2 {
				key, val := n.Content[i], n.Content[i+1]
				if seen[key.Value] {
					return NewValidationError("compose", "Composeに重複したキーがあります", "INVALID_COMPOSE")
				}
				seen[key.Value] = true
				if msg, ok := forbiddenComposeKeys[key.Value]; ok {
					return NewValidationError("compose."+key.Value, msg, "FORBIDDEN_COMPOSE_FEATURE")
				}
				if key.Value == "build" && val.Kind == yaml.MappingNode {
					for j := 0; j < len(val.Content); j += 2 {
						if val.Content[j].Value == "context" || val.Content[j].Value == "dockerfile" {
							if strings.Contains(val.Content[j+1].Value, "://") || strings.HasPrefix(val.Content[j+1].Value, "git@") {
								return NewValidationError("compose."+val.Content[j].Value, "リモートbuild contextは許可されていません", "PATH_OUTSIDE_PROJECT_ROOT")
							}
							if err := ValidateProjectPath(root, val.Content[j+1].Value); err != nil {
								return err
							}
						}
					}
				}
				if err := walkCompose([]*yaml.Node{val}, root); err != nil {
					return err
				}
			}
		}
		if n.Kind == yaml.SequenceNode {
			if err := walkCompose(n.Content, root); err != nil {
				return err
			}
		}
	}
	return nil
}

func ValidateEffectiveCompose(data []byte, publicService string, publicPort int) error {
	return validateEffectiveCompose(data, publicService, publicPort, "")
}

func ValidateEffectiveComposeWithOwnedNetwork(data []byte, publicService string, publicPort int, ownedNetwork string) error {
	return validateEffectiveCompose(data, publicService, publicPort, ownedNetwork)
}

func validateEffectiveCompose(data []byte, publicService string, publicPort int, ownedNetwork string) error {
	if publicPort < 1 || publicPort > 65535 {
		return NewValidationError("public.port", "公開portが不正です", "INVALID_PUBLIC_PORT")
	}
	var model map[string]any
	if err := json.Unmarshal(data, &model); err != nil {
		return NewValidationError("compose", "実効Compose JSONが不正です", "INVALID_COMPOSE")
	}
	services, ok := model["services"].(map[string]any)
	if !ok {
		return NewValidationError("services", "実効Composeにservicesがありません", "INVALID_COMPOSE")
	}
	if err := rejectExternalResources(model, ownedNetwork); err != nil {
		return err
	}
	if _, ok := services[publicService].(map[string]any); !ok {
		return NewValidationError("public.service", "指定されたserviceが存在しません", "SERVICE_NOT_FOUND")
	}
	for name, raw := range services {
		service, ok := raw.(map[string]any)
		if !ok {
			return NewValidationError("services."+name, "service定義が不正です", "INVALID_COMPOSE")
		}
		if err := validateEffectiveService(name, service); err != nil {
			return err
		}
	}
	return nil
}

func validateEffectiveService(name string, service map[string]any) error {
	field := "services." + name
	for _, key := range []string{"privileged", "tmpfs", "network_mode", "pid", "ipc", "ports"} {
		value, exists := service[key]
		if !exists {
			continue
		}
		if key == "network_mode" || key == "pid" || key == "ipc" {
			if value == "host" {
				return NewValidationError(field+"."+key, "host namespaceは許可されていません", "FORBIDDEN_COMPOSE_FEATURE")
			}
			continue
		}
		if key == "ports" {
			if ports, ok := value.([]any); ok && len(ports) == 0 {
				continue
			}
			return NewValidationError(field+".ports", "host portは許可されていません", "BIND_MOUNT_FORBIDDEN")
		}
		if enabled, ok := value.(bool); !ok || enabled {
			return NewValidationError(field+"."+key, "禁止された実効Composeです", "FORBIDDEN_COMPOSE_FEATURE")
		}
	}
	if volumes, ok := service["volumes"].([]any); ok {
		for _, volume := range volumes {
			if isBindMount(volume) {
				return NewValidationError(field+".volumes", "bind mountまたは匿名volumeは許可されていません", "BIND_MOUNT_FORBIDDEN")
			}
		}
	}
	return nil
}

// DeviceAttachment is the application-facing part of a normal Compose devices entry.
// Source is deliberately only a hint; LWS resolves it to a registered pool device at runtime.
type DeviceAttachment struct{ Service, Source, Target, Permissions string }

func ComposeDeviceAttachments(data []byte) ([]DeviceAttachment, error) {
	var model struct {
		Services map[string]struct {
			Devices []any `yaml:"devices"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &model); err != nil {
		return nil, NewValidationError("compose", "Compose YAMLが不正です", "INVALID_COMPOSE")
	}
	var result []DeviceAttachment
	for service, definition := range model.Services {
		for _, raw := range definition.Devices {
			var source, target, permissions string
			switch v := raw.(type) {
			case string:
				parts := strings.Split(v, ":")
				if len(parts) < 2 || len(parts) > 3 {
					return nil, NewValidationError("services."+service+".devices", "device指定が不正です", "INVALID_DEVICE")
				}
				source, target = parts[0], parts[1]
				if len(parts) == 3 {
					permissions = parts[2]
				}
			case map[string]any:
				source, _ = v["source"].(string)
				target, _ = v["target"].(string)
				permissions, _ = v["permissions"].(string)
			default:
				return nil, NewValidationError("services."+service+".devices", "device指定が不正です", "INVALID_DEVICE")
			}
			if !strings.HasPrefix(source, "/dev/") || !strings.HasPrefix(target, "/dev/") || strings.Contains(source, "..") || strings.Contains(target, "..") {
				return nil, NewValidationError("services."+service+".devices", "deviceは/dev配下の絶対pathで指定してください", "INVALID_DEVICE")
			}
			if permissions == "" {
				permissions = "rwm"
			}
			for _, c := range permissions {
				if !strings.ContainsRune("rwm", c) {
					return nil, NewValidationError("services."+service+".devices", "device権限が不正です", "INVALID_DEVICE")
				}
			}
			result = append(result, DeviceAttachment{service, source, target, permissions})
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Service == result[j].Service {
			return result[i].Target < result[j].Target
		}
		return result[i].Service < result[j].Service
	})
	return result, nil
}

func rejectExternalResources(model map[string]any, ownedNetwork string) error {
	for _, key := range []string{"volumes", "networks"} {
		resources, ok := model[key].(map[string]any)
		if !ok {
			continue
		}
		for name, raw := range resources {
			resource, ok := raw.(map[string]any)
			if external, _ := resource["external"].(bool); ok && external {
				resourceName, _ := resource["name"].(string)
				if key == "networks" && ownedNetwork != "" && name == "lws-edge" && resourceName == ownedNetwork {
					continue
				}
				return NewValidationError(key+"."+name, "外部resourceは許可されていません", "FORBIDDEN_EXTERNAL_RESOURCE")
			}
		}
	}
	return nil
}

func isBindMount(volume any) bool {
	switch value := volume.(type) {
	case string:
		parts := splitVolumeSpec(value)
		if len(parts) == 1 {
			return true
		}
		source := parts[0]
		return !strings.Contains(value, ":") || strings.Contains(source, "${") || source == "." || source == ".." || strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") || filepath.IsAbs(source) || strings.HasPrefix(source, "~")
	case map[string]any:
		kind, _ := value["type"].(string)
		return kind != "volume" || value["source"] == nil
	default:
		return true
	}
}

func splitVolumeSpec(value string) []string {
	parts := []string{}
	start := 0
	depth := 0
	for index, char := range value {
		switch char {
		case '{':
			depth++
		case '}':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 {
				parts = append(parts, value[start:index])
				start = index + 1
			}
		}
	}
	return append(parts, value[start:])
}

func ProjectPath(root, candidate string) string {
	return filepath.Join(root, filepath.Clean(candidate))
}

func NamedVolumeNames(data []byte) ([]string, error) {
	var model struct {
		Services map[string]struct {
			Volumes []any `yaml:"volumes"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &model); err != nil {
		return nil, NewValidationError("compose", "Compose YAMLが不正です", "INVALID_COMPOSE")
	}
	names := map[string]bool{}
	for _, service := range model.Services {
		for _, raw := range service.Volumes {
			switch value := raw.(type) {
			case string:
				parts := splitVolumeSpec(value)
				if len(parts) == 2 && !isBindMount(parts[0]+":/target") {
					names[parts[0]] = true
				}
			case map[string]any:
				if kind, _ := value["type"].(string); kind == "volume" {
					if name, _ := value["source"].(string); name != "" {
						names[name] = true
					}
				}
			}
		}
	}
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func ComposeServiceNames(data []byte) ([]string, error) {
	var model struct {
		Services map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &model); err != nil {
		return nil, NewValidationError("compose", "Compose YAMLが不正です", "INVALID_COMPOSE")
	}
	if len(model.Services) == 0 {
		return nil, NewValidationError("services", "Composeにserviceがありません", "INVALID_COMPOSE")
	}
	names := make([]string, 0, len(model.Services))
	for name := range model.Services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// ComposeServiceNetworkNames returns the networks already used by a service.
// The generated public-network override must keep these connections so that
// internal service discovery (for example web -> api) continues to work.
func ComposeServiceNetworkNames(data []byte, service string) ([]string, error) {
	var model struct {
		Services map[string]struct {
			Networks any `yaml:"networks"`
		} `yaml:"services"`
	}
	if err := yaml.Unmarshal(data, &model); err != nil {
		return nil, NewValidationError("compose", "Compose YAMLが不正です", "INVALID_COMPOSE")
	}
	item, ok := model.Services[service]
	if !ok {
		return nil, NewValidationError("services."+service, "公開serviceがComposeにありません", "INVALID_COMPOSE")
	}
	names := []string{}
	switch networks := item.Networks.(type) {
	case map[string]any:
		for name := range networks {
			names = append(names, name)
		}
	case []any:
		for _, value := range networks {
			if name, ok := value.(string); ok {
				names = append(names, name)
			}
		}
	default:
		// Compose creates the implicit default network when none is declared.
		names = append(names, "default")
	}
	sort.Strings(names)
	return names, nil
}

// ComposeServiceNetworkNamesFromSources resolves a service's networks from
// the last Compose source that defines it, preserving override semantics.
func ComposeServiceNetworkNamesFromSources(sources []ComposeSource, service string) ([]string, error) {
	for i := len(sources) - 1; i >= 0; i-- {
		if names, err := ComposeServiceNetworkNames(sources[i].Data, service); err == nil {
			return names, nil
		}
	}
	return nil, NewValidationError("services."+service, "公開serviceがComposeにありません", "INVALID_COMPOSE")
}

func ComposeNetworkNames(data []byte, project string) ([]string, error) {
	var model struct {
		Networks map[string]struct {
			Name string `yaml:"name"`
		} `yaml:"networks"`
	}
	if err := yaml.Unmarshal(data, &model); err != nil {
		return nil, NewValidationError("compose", "Compose YAMLが不正です", "INVALID_COMPOSE")
	}
	if len(model.Networks) == 0 {
		return []string{project + "_default"}, nil
	}
	names := make([]string, 0, len(model.Networks))
	for key, network := range model.Networks {
		if network.Name != "" {
			names = append(names, network.Name)
		} else {
			names = append(names, project+"_"+key)
		}
	}
	sort.Strings(names)
	return names, nil
}

func ComposeError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("Compose検証に失敗しました: %s", err)
}
