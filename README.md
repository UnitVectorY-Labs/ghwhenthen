[![License](https://img.shields.io/badge/license-MIT-blue.svg)](https://opensource.org/licenses/MIT) [![Work In Progress](https://img.shields.io/badge/Status-Work%20In%20Progress-yellow)](https://guide.unitvectorylabs.com/bestpractices/status/#work-in-progress) [![Go Report Card](https://goreportcard.com/badge/github.com/UnitVectorY-Labs/ghwhenthen)](https://goreportcard.com/report/github.com/UnitVectorY-Labs/ghwhenthen) 

# ghwhenthen

A lightweight, configuration-driven service that consumes GitHub webhook events from Google Cloud Pub/Sub, evaluates declarative rules against message attributes and payloads, and executes ordered GitHub GraphQL actions.

## Key Features

- **Pub/Sub Consumer** — Pulls GitHub webhook events from a Google Cloud Pub/Sub subscription with gzip decompression support
- **Declarative Rules** — Define when/then rules using a simple expression language (`=`, `!=`, `AND`, `OR`, `has()`, parentheses)
- **GitHub GraphQL Actions** — Execute ordered sequences of GraphQL mutations with variable interpolation and step output chaining
- **Configuration-Driven** — All behavior defined in a single YAML file
- **Kubernetes Ready** — Health endpoints, structured JSON logging, graceful shutdown

## Quick Start

### Build

```bash
go build -o ghwhenthen .
```

### Configure

Create a YAML config file (see [examples/config.yaml](examples/config.yaml)):

```yaml
pubsub:
  project_id: my-gcp-project
  subscription_id: github-webhook-events
github:
  token_env_var: GITHUB_TOKEN
behavior:
  on_failure: nack
rules:
  - name: my-rule
    enabled: true
    when: 'attributes.gh_event = "pull_request" AND attributes.action = "opened"'
    then:
      - name: add_to_project
        type: github_graphql
        config:
          document: |
            mutation { ... }
          variables:
            projectId: ${constants.projects.public.project_id}
```

### Run

```bash
export GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx
ghwhenthen --config config.yaml
```

## Documentation

Full documentation is available at the [ghwhenthen docs site](https://ghwhenthen.unitvectorylabs.com/), including:

- [Command Line](https://ghwhenthen.unitvectorylabs.com/COMMAND.html) — Flags and environment variables
- [Configuration](https://ghwhenthen.unitvectorylabs.com/CONFIGURATION.html) — Complete YAML config reference
- [Examples](https://ghwhenthen.unitvectorylabs.com/EXAMPLES.html) — Real-world use cases
