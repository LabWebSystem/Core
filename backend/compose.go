package backend

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var forbiddenComposeKeys = map[string]string{"include": "外部Composeのincludeは許可されていません", "extends": "Composeのextendsは許可されていません", "env_file": "env_fileは許可されていません", "label_file": "label_fileは許可されていません", "volumes_from": "volumes_fromは許可されていません", "privileged": "privilegedは許可されていません", "devices": "deviceは許可されていません", "network_mode": "host networkは許可されていません", "pid": "host PIDは許可されていません", "ipc": "host IPCは許可されていません", "tmpfs": "tmpfsは許可されていません", "configs": "ファイル型configsは許可されていません", "secrets": "ファイル型secretsは許可されていません", "additional_contexts": "追加build contextは許可されていません"}

func ValidateComposeSource(root string, data []byte) error {
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return NewValidationError("compose", "Compose YAMLが不正です", "INVALID_COMPOSE")
	}
	if err := walkCompose(node.Content, root); err != nil {
		return err
	}
	return nil
}

func walkCompose(nodes []*yaml.Node, root string) error {
	for _, n := range nodes {
		if n.Kind == yaml.AliasNode || n.Anchor != "" || n.Tag != "" && strings.HasPrefix(n.Tag, "!") {
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
	for _, key := range []string{"privileged", "devices", "tmpfs", "network_mode", "pid", "ipc", "ports"} {
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
		source := strings.SplitN(value, ":", 2)[0]
		return !strings.Contains(value, ":") || source == "." || source == ".." || strings.HasPrefix(source, "./") || strings.HasPrefix(source, "../") || filepath.IsAbs(source) || strings.HasPrefix(source, "~")
	case map[string]any:
		kind, _ := value["type"].(string)
		return kind != "volume" || value["source"] == nil
	default:
		return true
	}
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
				parts := strings.SplitN(value, ":", 2)
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

func ComposeError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("Compose検証に失敗しました: %s", err)
}
