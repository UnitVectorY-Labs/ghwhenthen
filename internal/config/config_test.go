package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = `
pubsub:
  project_id: my-project
  subscription_id: my-sub

github:
  token_env_var: GH_TOKEN
  graphql_endpoint: https://api.github.com/graphql

behavior:
  on_failure: ack

constants:
  label: bug

rules:
  - name: rule1
    enabled: true
    when: "event.action == 'opened'"
    then:
      - name: step1
        type: github_graphql
        config:
          document: "mutation { addLabel }"
          variables:
            id: "123"
          outputs:
            result: "data.addLabel"
`

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadValidConfig(t *testing.T) {
	path := writeYAML(t, validYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.PubSub.ProjectID != "my-project" {
		t.Errorf("ProjectID = %q, want %q", cfg.PubSub.ProjectID, "my-project")
	}
	if cfg.PubSub.SubscriptionID != "my-sub" {
		t.Errorf("SubscriptionID = %q, want %q", cfg.PubSub.SubscriptionID, "my-sub")
	}
	if cfg.GitHub.TokenEnvVar != "GH_TOKEN" {
		t.Errorf("TokenEnvVar = %q, want %q", cfg.GitHub.TokenEnvVar, "GH_TOKEN")
	}
	if cfg.GitHub.GraphQLEndpoint != "https://api.github.com/graphql" {
		t.Errorf("GraphQLEndpoint = %q, want %q", cfg.GitHub.GraphQLEndpoint, "https://api.github.com/graphql")
	}
	if cfg.Behavior.OnFailure != "ack" {
		t.Errorf("OnFailure = %q, want %q", cfg.Behavior.OnFailure, "ack")
	}
	if len(cfg.Rules) != 1 {
		t.Fatalf("len(Rules) = %d, want 1", len(cfg.Rules))
	}
	if cfg.Rules[0].Name != "rule1" {
		t.Errorf("Rules[0].Name = %q, want %q", cfg.Rules[0].Name, "rule1")
	}
	if cfg.Rules[0].Then[0].Config.Document != "mutation { addLabel }" {
		t.Errorf("Document = %q, want %q", cfg.Rules[0].Then[0].Config.Document, "mutation { addLabel }")
	}
	if cfg.Rules[0].Then[0].Config.Variables["id"] != "123" {
		t.Errorf("Variables[id] = %q, want %q", cfg.Rules[0].Then[0].Config.Variables["id"], "123")
	}
	if cfg.Rules[0].Then[0].Config.Outputs["result"] != "data.addLabel" {
		t.Errorf("Outputs[result] = %q, want %q", cfg.Rules[0].Then[0].Config.Outputs["result"], "data.addLabel")
	}
}

func TestLoadFileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestValidateValid(t *testing.T) {
	path := writeYAML(t, validYAML)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	t.Setenv("GH_TOKEN", "ghp_test123")
	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() error: %v", err)
	}
}

func TestValidateMissingProjectID(t *testing.T) {
	yaml := `
pubsub:
  project_id: ""
  subscription_id: my-sub
github:
  token_env_var: GH_TOKEN
behavior:
  on_failure: ack
rules:
  - name: r
    when: "true"
    then:
      - name: s
        type: github_graphql
        config:
          document: "query {}"
`
	path := writeYAML(t, yaml)
	cfg, _ := Load(path)
	t.Setenv("GH_TOKEN", "ghp_test")
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "pubsub.project_id") {
		t.Fatalf("expected pubsub.project_id error, got: %v", err)
	}
}

func TestValidateMissingSubscriptionID(t *testing.T) {
	yaml := `
pubsub:
  project_id: proj
  subscription_id: ""
github:
  token_env_var: GH_TOKEN
behavior:
  on_failure: ack
rules:
  - name: r
    when: "true"
    then:
      - name: s
        type: github_graphql
        config:
          document: "query {}"
`
	path := writeYAML(t, yaml)
	cfg, _ := Load(path)
	t.Setenv("GH_TOKEN", "ghp_test")
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "pubsub.subscription_id") {
		t.Fatalf("expected pubsub.subscription_id error, got: %v", err)
	}
}

func TestValidateMissingTokenEnvVar(t *testing.T) {
	yaml := `
pubsub:
  project_id: proj
  subscription_id: sub
github:
  token_env_var: ""
behavior:
  on_failure: ack
rules:
  - name: r
    when: "true"
    then:
      - name: s
        type: github_graphql
        config:
          document: "query {}"
`
	path := writeYAML(t, yaml)
	cfg, _ := Load(path)
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "github.token_env_var") {
		t.Fatalf("expected github.token_env_var error, got: %v", err)
	}
}

func TestValidateInvalidOnFailure(t *testing.T) {
	yaml := `
pubsub:
  project_id: proj
  subscription_id: sub
github:
  token_env_var: GH_TOKEN
behavior:
  on_failure: retry
rules:
  - name: r
    when: "true"
    then:
      - name: s
        type: github_graphql
        config:
          document: "query {}"
`
	path := writeYAML(t, yaml)
	cfg, _ := Load(path)
	t.Setenv("GH_TOKEN", "ghp_test")
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "behavior.on_failure") {
		t.Fatalf("expected behavior.on_failure error, got: %v", err)
	}
}

func TestValidateNoRules(t *testing.T) {
	yaml := `
pubsub:
  project_id: proj
  subscription_id: sub
github:
  token_env_var: GH_TOKEN
behavior:
  on_failure: ack
rules: []
`
	path := writeYAML(t, yaml)
	cfg, _ := Load(path)
	t.Setenv("GH_TOKEN", "ghp_test")
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "at least one rule") {
		t.Fatalf("expected 'at least one rule' error, got: %v", err)
	}
}

func TestValidateRuleMissingName(t *testing.T) {
	yaml := `
pubsub:
  project_id: proj
  subscription_id: sub
github:
  token_env_var: GH_TOKEN
behavior:
  on_failure: ack
rules:
  - name: ""
    when: "true"
    then:
      - name: s
        type: github_graphql
        config:
          document: "query {}"
`
	path := writeYAML(t, yaml)
	cfg, _ := Load(path)
	t.Setenv("GH_TOKEN", "ghp_test")
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "rules[0].name") {
		t.Fatalf("expected rules[0].name error, got: %v", err)
	}
}

func TestValidateRuleMissingWhen(t *testing.T) {
	yaml := `
pubsub:
  project_id: proj
  subscription_id: sub
github:
  token_env_var: GH_TOKEN
behavior:
  on_failure: ack
rules:
  - name: r
    when: ""
    then:
      - name: s
        type: github_graphql
        config:
          document: "query {}"
`
	path := writeYAML(t, yaml)
	cfg, _ := Load(path)
	t.Setenv("GH_TOKEN", "ghp_test")
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "rules[0].when") {
		t.Fatalf("expected rules[0].when error, got: %v", err)
	}
}

func TestValidateRuleNoSteps(t *testing.T) {
	yaml := `
pubsub:
  project_id: proj
  subscription_id: sub
github:
  token_env_var: GH_TOKEN
behavior:
  on_failure: ack
rules:
  - name: r
    when: "true"
    then: []
`
	path := writeYAML(t, yaml)
	cfg, _ := Load(path)
	t.Setenv("GH_TOKEN", "ghp_test")
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "rules[0].then") {
		t.Fatalf("expected rules[0].then error, got: %v", err)
	}
}

func TestValidateStepMissingName(t *testing.T) {
	yaml := `
pubsub:
  project_id: proj
  subscription_id: sub
github:
  token_env_var: GH_TOKEN
behavior:
  on_failure: ack
rules:
  - name: r
    when: "true"
    then:
      - name: ""
        type: github_graphql
        config:
          document: "query {}"
`
	path := writeYAML(t, yaml)
	cfg, _ := Load(path)
	t.Setenv("GH_TOKEN", "ghp_test")
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "then[0].name") {
		t.Fatalf("expected then[0].name error, got: %v", err)
	}
}

func TestValidateStepMissingType(t *testing.T) {
	yaml := `
pubsub:
  project_id: proj
  subscription_id: sub
github:
  token_env_var: GH_TOKEN
behavior:
  on_failure: ack
rules:
  - name: r
    when: "true"
    then:
      - name: s
        type: ""
        config:
          document: "query {}"
`
	path := writeYAML(t, yaml)
	cfg, _ := Load(path)
	t.Setenv("GH_TOKEN", "ghp_test")
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "then[0].type") {
		t.Fatalf("expected then[0].type error, got: %v", err)
	}
}

func TestValidateStepMissingDocument(t *testing.T) {
	yaml := `
pubsub:
  project_id: proj
  subscription_id: sub
github:
  token_env_var: GH_TOKEN
behavior:
  on_failure: ack
rules:
  - name: r
    when: "true"
    then:
      - name: s
        type: github_graphql
        config:
          document: ""
`
	path := writeYAML(t, yaml)
	cfg, _ := Load(path)
	t.Setenv("GH_TOKEN", "ghp_test")
	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "config.document") {
		t.Fatalf("expected config.document error, got: %v", err)
	}
}

func TestValidateDefaultGraphQLEndpoint(t *testing.T) {
	yaml := `
pubsub:
  project_id: proj
  subscription_id: sub
github:
  token_env_var: GH_TOKEN
behavior:
  on_failure: nack
rules:
  - name: r
    when: "true"
    then:
      - name: s
        type: github_graphql
        config:
          document: "query {}"
`
	path := writeYAML(t, yaml)
	cfg, _ := Load(path)
	t.Setenv("GH_TOKEN", "ghp_test")

	if cfg.GitHub.GraphQLEndpoint != "" {
		t.Fatalf("expected empty GraphQLEndpoint before Validate, got %q", cfg.GitHub.GraphQLEndpoint)
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() error: %v", err)
	}

	if cfg.GitHub.GraphQLEndpoint != "https://api.github.com/graphql" {
		t.Fatalf("GraphQLEndpoint = %q, want %q", cfg.GitHub.GraphQLEndpoint, "https://api.github.com/graphql")
	}
}

func TestValidateTokenEnvVarNotSet(t *testing.T) {
	yaml := `
pubsub:
  project_id: proj
  subscription_id: sub
github:
  token_env_var: NONEXISTENT_TOKEN_VAR
behavior:
  on_failure: ack
rules:
  - name: r
    when: "true"
    then:
      - name: s
        type: github_graphql
        config:
          document: "query {}"
`
	path := writeYAML(t, yaml)
	cfg, _ := Load(path)

	// Ensure the env var is truly unset
	os.Unsetenv("NONEXISTENT_TOKEN_VAR")

	err := Validate(cfg)
	if err == nil || !strings.Contains(err.Error(), "NONEXISTENT_TOKEN_VAR") {
		t.Fatalf("expected env var error, got: %v", err)
	}
}

func TestValidateTokenEnvVarSet(t *testing.T) {
	yaml := `
pubsub:
  project_id: proj
  subscription_id: sub
github:
  token_env_var: MY_GH_TOKEN
behavior:
  on_failure: ack
rules:
  - name: r
    when: "true"
    then:
      - name: s
        type: github_graphql
        config:
          document: "query {}"
`
	path := writeYAML(t, yaml)
	cfg, _ := Load(path)
	t.Setenv("MY_GH_TOKEN", "ghp_valid_token")

	if err := Validate(cfg); err != nil {
		t.Fatalf("Validate() error: %v", err)
	}
}
