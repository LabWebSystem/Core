package backend

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

var subdomainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var variablePattern = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)
var servicePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
var baseDomainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)

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
	if err := yaml.Unmarshal(data, &m); err != nil || m.APIVersion != "lws/v1" || m.Metadata.Name == "" || len([]rune(m.Metadata.Name)) > 80 || len([]rune(m.Metadata.Description)) > 500 || !servicePattern.MatchString(m.Public.Service) || m.Public.Port < 1 || m.Public.Port > 65535 {
		return Manifest{}, NewValidationError("manifest", "manifestのschemaが不正です", "INVALID_MANIFEST")
	}
	return m, nil
}

func inspectManifestNode(root *yaml.Node) error {
	allowed := map[string]bool{"apiVersion": true, "metadata": true, "public": true}
	seen := map[string]bool{}
	for i := 0; i < len(root.Content); i += 2 {
		k, v := root.Content[i], root.Content[i+1]
		if k.Tag != "!!str" || seen[k.Value] || !allowed[k.Value] || v.Anchor != "" || v.Kind == yaml.AliasNode || v.Tag != "!!map" && v.Tag != "!!str" && v.Tag != "!!int" {
			return NewValidationError("manifest", "manifestに未許可のYAML構文があります", "INVALID_MANIFEST")
		}
		seen[k.Value] = true
	}
	return nil
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
