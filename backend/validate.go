package backend

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

var subdomainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var variablePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
var servicePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
var baseDomainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
var composeVariablePattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?:(:?[-?])(.*))?\}`)
var simpleComposeVariablePattern = regexp.MustCompile(`(^|[^$])\$([A-Za-z_][A-Za-z0-9_]*)`)

func ValidateBaseDomain(value string) error {
	if !baseDomainPattern.MatchString(value) {
		return NewValidationError("baseDomain", "ベースドメインが不正です", "INVALID_BASE_DOMAIN")
	}
	return nil
}

func ValidateRepositoryURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path == "" {
		return NewValidationError("repositoryUrl", "GitHubのHTTPSリポジトリURLだけを指定してください", "INVALID_REPOSITORY_URL")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(parts[1], "\\") {
		return NewValidationError("repositoryUrl", "GitHubのHTTPSリポジトリURLだけを指定してください", "INVALID_REPOSITORY_URL")
	}
	return nil
}

func ValidateSubdomain(value string) error {
	if !subdomainPattern.MatchString(value) {
		return NewValidationError("subdomain", "subdomainが不正です", "INVALID_SUBDOMAIN")
	}
	return nil
}

func ValidateVariableName(value string) error {
	if !variablePattern.MatchString(value) {
		return NewValidationError("name", "環境変数名が不正です", "INVALID_ENVIRONMENT_VARIABLE")
	}
	return nil
}

func ValidateInstallationID(value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("installation IDが設定されていません")
	}
	return nil
}

func ValidateRequestID(value string) error {
	id, err := uuid.Parse(value)
	if err != nil || id.Version() != 4 {
		return NewValidationError("requestId", "requestIdはUUID v4で指定してください", "INVALID_REQUEST_ID")
	}
	return nil
}

type ComposeVariable struct {
	Name       string
	Required   bool
	HasDefault bool
}

func ValidateComposeVariableValues(variables []ComposeVariable, values map[string]string) error {
	for name, value := range values {
		if err := ValidateVariableName(name); err != nil {
			return err
		}
		if strings.ContainsRune(value, '\x00') {
			return NewValidationError(name, "環境変数の値にNUL文字は指定できません", "INVALID_ENVIRONMENT_VARIABLE")
		}
	}
	for _, variable := range variables {
		if variable.Required && values[variable.Name] == "" {
			return NewValidationError(variable.Name, "必須の環境変数が指定されていません", "MISSING_REQUIRED_ENVIRONMENT_VARIABLE")
		}
	}
	return nil
}

func ExtractComposeVariables(data []byte) ([]ComposeVariable, error) {
	text := string(data)
	variables := map[string]ComposeVariable{}
	for offset := 0; offset < len(text); {
		start := strings.Index(text[offset:], "${")
		if start < 0 {
			break
		}
		start += offset
		end := strings.IndexByte(text[start+2:], '}')
		if end < 0 {
			return nil, NewValidationError("compose", "Composeの変数参照が閉じられていません", "INVALID_ENVIRONMENT_VARIABLE")
		}
		end += start + 2
		match := composeVariablePattern.FindStringSubmatch(text[start : end+1])
		if match == nil {
			return nil, NewValidationError("compose", "Composeの変数参照が不正です", "INVALID_ENVIRONMENT_VARIABLE")
		}
		if err := ValidateVariableName(match[1]); err != nil {
			return nil, err
		}
		hasDefault := match[2] == "-" || match[2] == ":-"
		variables[match[1]] = ComposeVariable{Name: match[1], Required: !hasDefault, HasDefault: hasDefault}
		offset = end + 1
	}
	for _, match := range simpleComposeVariablePattern.FindAllStringSubmatch(text, -1) {
		if err := ValidateVariableName(match[2]); err != nil {
			return nil, err
		}
		variables[match[2]] = ComposeVariable{Name: match[2], Required: true}
	}
	result := make([]ComposeVariable, 0, len(variables))
	for _, variable := range variables {
		result = append(result, variable)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

type Manifest struct {
	APIVersion string `yaml:"apiVersion"`
	Metadata   struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	} `yaml:"metadata"`
	Public struct {
		Service string `yaml:"service"`
		Port    int    `yaml:"port"`
	} `yaml:"public"`
}

func ValidateManifest(data []byte) (Manifest, error) {
	var m Manifest
	if len(data) == 0 || len(data) > 8192 || !utf8.Valid(data) {
		return m, NewValidationError("manifest", "manifestがUTF-8の8KiB以内ではありません", "INVALID_MANIFEST")
	}
	n := yaml.Node{}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	if err := dec.Decode(&n); err != nil {
		return m, NewValidationError("manifest", "manifestのYAMLが不正です", "INVALID_MANIFEST")
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err == nil {
		return m, NewValidationError("manifest", "manifestは単一documentで指定してください", "INVALID_MANIFEST")
	}
	if n.Kind != yaml.DocumentNode || len(n.Content) != 1 || n.Content[0].Kind != yaml.MappingNode {
		return m, NewValidationError("manifest", "manifestはobjectで指定してください", "INVALID_MANIFEST")
	}
	if err := inspectManifestNode(n.Content[0]); err != nil {
		return m, err
	}
	if err := yaml.Unmarshal(data, &m); err != nil || m.APIVersion != "lws/v1" || m.Metadata.Name == "" || strings.TrimSpace(m.Metadata.Name) != m.Metadata.Name || len([]rune(m.Metadata.Name)) > 80 || len([]rune(m.Metadata.Description)) > 500 || strings.ContainsAny(m.Metadata.Description, "\r\n\x00") || !servicePattern.MatchString(m.Public.Service) || m.Public.Port < 1 || m.Public.Port > 65535 {
		return Manifest{}, NewValidationError("manifest", "manifestのschemaが不正です", "INVALID_MANIFEST")
	}
	return m, nil
}

func inspectManifestNode(root *yaml.Node) error {
	if err := inspectManifestMapping(root, map[string]string{"apiVersion": "str", "metadata": "map", "public": "map"}, map[string]bool{"apiVersion": true, "metadata": true, "public": true}); err != nil {
		return err
	}
	metadata, public := findManifestValues(root, "metadata", "public")
	if err := inspectManifestMapping(metadata, map[string]string{"name": "str", "description": "str"}, map[string]bool{"name": true, "description": true}); err != nil {
		return err
	}
	return inspectManifestMapping(public, map[string]string{"service": "str", "port": "int"}, map[string]bool{"service": true, "port": true})
}

func inspectManifestMapping(node *yaml.Node, allowed map[string]string, required map[string]bool) error {
	if node == nil || node.Kind != yaml.MappingNode {
		return NewValidationError("manifest", "manifestのschemaが不正です", "INVALID_MANIFEST")
	}
	seen := map[string]bool{}
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		kind, ok := allowed[key.Value]
		if key.Tag != "!!str" || seen[key.Value] || !ok || value.Anchor != "" || value.Kind == yaml.AliasNode {
			return NewValidationError("manifest", "manifestに未許可のYAML構文があります", "INVALID_MANIFEST")
		}
		if (kind == "map" && value.Kind != yaml.MappingNode) || (kind == "str" && value.Tag != "!!str") || (kind == "int" && value.Tag != "!!int") {
			return NewValidationError("manifest", "manifestのschemaが不正です", "INVALID_MANIFEST")
		}
		seen[key.Value] = true
	}
	for key := range required {
		if !seen[key] {
			return NewValidationError("manifest", "manifestのschemaが不正です", "INVALID_MANIFEST")
		}
	}
	return nil
}

func findManifestValues(root *yaml.Node, names ...string) (*yaml.Node, *yaml.Node) {
	values := make(map[string]*yaml.Node, len(names))
	for i := 0; i+1 < len(root.Content); i += 2 {
		for _, name := range names {
			if root.Content[i].Value == name {
				values[name] = root.Content[i+1]
			}
		}
	}
	return values[names[0]], values[names[1]]
}

func ValidateProjectPath(root, candidate string) error {
	if filepath.IsAbs(candidate) {
		return NewValidationError("path", fmt.Sprintf("指定されたパス %q は許可された範囲外です", candidate), "PATH_OUTSIDE_PROJECT_ROOT")
	}
	clean := filepath.Clean(filepath.Join(root, candidate))
	rel, err := filepath.Rel(root, clean)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return NewValidationError("path", fmt.Sprintf("指定されたパス %q は許可された範囲外です", candidate), "PATH_OUTSIDE_PROJECT_ROOT")
	}
	return nil
}

type ValidationError struct{ Field, Message, Reason string }

func (e *ValidationError) Error() string { return e.Message }
func NewValidationError(field, message, reason string) *ValidationError {
	return &ValidationError{field, message, reason}
}
