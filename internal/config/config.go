package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config is the top-level configuration structure.
type Config struct {
	PubSub    PubSubConfig           `yaml:"pubsub"`
	GitHub    GitHubConfig           `yaml:"github"`
	Behavior  BehaviorConfig         `yaml:"behavior"`
	Constants map[string]interface{} `yaml:"constants"`
	Rules     []Rule                 `yaml:"rules"`
}

// PubSubConfig holds Google Cloud Pub/Sub settings.
type PubSubConfig struct {
	ProjectID      string `yaml:"project_id"`
	SubscriptionID string `yaml:"subscription_id"`
}

// GitHubConfig holds GitHub API settings.
type GitHubConfig struct {
	TokenEnvVar     string `yaml:"token_env_var"`
	GraphQLEndpoint string `yaml:"graphql_endpoint"`
}

// BehaviorConfig controls message handling behavior.
type BehaviorConfig struct {
	OnFailure string `yaml:"on_failure"`
}

// Rule defines a single automation rule.
type Rule struct {
	Name    string `yaml:"name"`
	Enabled bool   `yaml:"enabled"`
	When    string `yaml:"when"`
	Then    []Step `yaml:"then"`
}

// Step defines an action to execute when a rule matches.
type Step struct {
	Name   string     `yaml:"name"`
	Type   string     `yaml:"type"`
	Config StepConfig `yaml:"config"`
}

// StepConfig holds the configuration for a step.
type StepConfig struct {
	Document  string            `yaml:"document"`
	Variables map[string]string `yaml:"variables"`
	Outputs   map[string]string `yaml:"outputs"`
}

// Load reads a YAML configuration file from path and returns a Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return &cfg, nil
}

// Validate checks the Config for required fields and sets defaults.
func Validate(cfg *Config) error {
	if cfg.PubSub.ProjectID == "" {
		return fmt.Errorf("pubsub.project_id is required")
	}
	if cfg.PubSub.SubscriptionID == "" {
		return fmt.Errorf("pubsub.subscription_id is required")
	}

	if cfg.GitHub.TokenEnvVar == "" {
		return fmt.Errorf("github.token_env_var is required")
	}

	if cfg.Behavior.OnFailure != "ack" && cfg.Behavior.OnFailure != "nack" {
		return fmt.Errorf("behavior.on_failure must be \"ack\" or \"nack\"")
	}

	if len(cfg.Rules) == 0 {
		return fmt.Errorf("at least one rule is required")
	}

	for i, r := range cfg.Rules {
		if r.Name == "" {
			return fmt.Errorf("rules[%d].name is required", i)
		}
		if r.When == "" {
			return fmt.Errorf("rules[%d].when is required", i)
		}
		if len(r.Then) == 0 {
			return fmt.Errorf("rules[%d].then must have at least one step", i)
		}
		for j, s := range r.Then {
			if s.Name == "" {
				return fmt.Errorf("rules[%d].then[%d].name is required", i, j)
			}
			if s.Type == "" {
				return fmt.Errorf("rules[%d].then[%d].type is required", i, j)
			}
			if s.Type == "github_graphql" && s.Config.Document == "" {
				return fmt.Errorf("rules[%d].then[%d].config.document is required for type github_graphql", i, j)
			}
		}
	}

	// Apply default for graphql_endpoint
	if cfg.GitHub.GraphQLEndpoint == "" {
		cfg.GitHub.GraphQLEndpoint = "https://api.github.com/graphql"
	}

	// Verify the token env var is set
	if os.Getenv(cfg.GitHub.TokenEnvVar) == "" {
		return fmt.Errorf("environment variable %q (github.token_env_var) is not set or empty", cfg.GitHub.TokenEnvVar)
	}

	return nil
}
