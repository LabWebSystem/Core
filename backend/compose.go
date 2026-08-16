package backend

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

var forbiddenComposeKeys = map[string]string{"include": "外部Composeのincludeは許可されていません", "extends": "Composeのextendsは許可されていません", "env_file": "env_fileは許可されていません", "label_file": "label_fileは許可されていません", "volumes_from": "volumes_fromは許可されていません", "privileged": "privilegedは許可されていません", "devices": "deviceは許可されていません", "network_mode": "host networkは許可されていません", "pid": "host PIDは許可されていません", "ipc": "host IPCは許可されていません", "tmpfs": "tmpfsは許可されていません"}

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
			for i := 0; i < len(n.Content); i += 2 {
				key, val := n.Content[i], n.Content[i+1]
				if msg, ok := forbiddenComposeKeys[key.Value]; ok {
					return NewValidationError("compose."+key.Value, msg, "FORBIDDEN_COMPOSE_FEATURE")
				}
				if key.Value == "build" && val.Kind == yaml.MappingNode {
					for j := 0; j < len(val.Content); j += 2 {
						if val.Content[j].Value == "context" || val.Content[j].Value == "dockerfile" {
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

type EffectiveService struct {
	Image       string         `json:"image"`
	Privileged  bool           `json:"privileged"`
	NetworkMode string         `json:"network_mode"`
	Ports       []any          `json:"ports"`
	Volumes     []any          `json:"volumes"`
	Networks    map[string]any `json:"networks"`
}
type EffectiveCompose struct {
	Services map[string]EffectiveService `json:"services"`
}

func ValidateEffectiveCompose(data []byte, publicService string, publicPort int) error {
	var model EffectiveCompose
	if err := json.Unmarshal(data, &model); err != nil {
		return NewValidationError("compose", "実効Compose JSONが不正です", "INVALID_COMPOSE")
	}
	svc, ok := model.Services[publicService]
	if !ok {
		return NewValidationError("public.service", "指定されたserviceが存在しません", "SERVICE_NOT_FOUND")
	}
	if svc.Privileged || svc.NetworkMode == "host" {
		return NewValidationError("services."+publicService, "禁止された実効Composeです", "FORBIDDEN_COMPOSE_FEATURE")
	}
	if len(svc.Ports) > 0 || len(svc.Volumes) > 0 {
		return NewValidationError("services."+publicService, "host portまたはvolumeは許可されていません", "BIND_MOUNT_FORBIDDEN")
	}
	_ = publicPort
	return nil
}

func ProjectPath(root, candidate string) string {
	return filepath.Join(root, filepath.Clean(candidate))
}
func ComposeError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("Compose検証に失敗しました: %s", err)
}
