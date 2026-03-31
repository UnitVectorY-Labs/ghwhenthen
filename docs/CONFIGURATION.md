---
layout: default
title: Configuration
nav_order: 3
---

# Configuration

ghwhenthen uses a single YAML configuration file to define all behavior. The file is specified via the `--config` flag or `GHWHENTHEN_CONFIG` environment variable.

## Top-Level Structure

```yaml
pubsub:
  project_id: "..."
  subscription_id: "..."

github:
  token_env_var: "..."
  graphql_endpoint: "..."  # optional

behavior:
  on_failure: "..."

constants:
  # arbitrary nested map

rules:
  - name: "..."
    enabled: true
    when: "..."
    then:
      - name: "..."
        type: "..."
        config: { ... }
```

---

## `pubsub`

Configures the Google Cloud Pub/Sub subscription to consume messages from.

| Field | Required | Description |
|-------|----------|-------------|
| `project_id` | Yes | Google Cloud project ID that owns the subscription |
| `subscription_id` | Yes | Pub/Sub subscription ID to pull messages from |

```yaml
pubsub:
  project_id: my-gcp-project
  subscription_id: github-webhook-events
```

---

## `github`

Configures access to the GitHub API.

| Field | Required | Description | Default |
|-------|----------|-------------|---------|
| `token_env_var` | Yes | Name of the environment variable containing the GitHub PAT | — |
| `graphql_endpoint` | No | GitHub GraphQL API endpoint URL | `https://api.github.com/graphql` |

```yaml
github:
  token_env_var: GITHUB_TOKEN
  graphql_endpoint: https://api.github.com/graphql
```

The `graphql_endpoint` field is useful for GitHub Enterprise Server installations that use a different API URL.

---

## `behavior`

Controls how the service handles processing failures.

| Field | Required | Description |
|-------|----------|-------------|
| `on_failure` | Yes | Action to take when processing fails: `ack` or `nack` |

- **`ack`** — Acknowledge the message even on failure. The message will not be redelivered. Use this to prevent poison messages from blocking the subscription.
- **`nack`** — Negatively acknowledge the message on failure. Pub/Sub will redeliver the message according to the subscription's retry policy.

```yaml
behavior:
  on_failure: nack
```

---

## `constants`

An arbitrary nested map of reusable values. Constants are referenced in step variables using the `${constants.path.to.value}` syntax.

This is commonly used for GitHub Project V2 node IDs, field IDs, and option IDs that are referenced across multiple rules.

```yaml
constants:
  projects:
    public:
      project_id: PVT_kwDO_EXAMPLE
      status_field_id: PVTSSF_EXAMPLE_STATUS
      todo_option_id: f75ad846
    private:
      project_id: PVT_kwDO_PRIVATE
      status_field_id: PVTSSF_PRIVATE_STATUS
      todo_option_id: a1b2c3d4
```

Values are accessed with dot notation: `${constants.projects.public.project_id}`

---

## `rules`

A list of automation rules. Each rule is evaluated against every incoming Pub/Sub message. At least one rule is required.

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique name for the rule (used in logging) |
| `enabled` | Yes | Boolean — whether the rule is active |
| `when` | Yes | Expression string that determines if the rule matches |
| `then` | Yes | Ordered list of steps to execute when the rule matches |

```yaml
rules:
  - name: my-rule
    enabled: true
    when: 'attributes.gh_event = "push"'
    then:
      - name: step1
        type: github_graphql
        config: { ... }
```

---

## Expression Language (`when`)

The `when` field uses a simple expression language to match against event data.

### Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `=` | Equality | `attributes.action = "opened"` |
| `!=` | Inequality | `payload.repository.private != true` |
| `AND` | Logical AND | `a = "x" AND b = "y"` |
| `OR` | Logical OR | `a = "x" OR a = "y"` |
| `( )` | Grouping | `(a = "x" OR a = "y") AND b = "z"` |

### Literal Types

| Type | Syntax | Example |
|------|--------|---------|
| String | Double-quoted | `"opened"`, `"public"` |
| Boolean | `true` / `false` | `payload.repository.private = false` |
| Null | `null` | `payload.field = null` |

### Functions

| Function | Description | Example |
|----------|-------------|---------|
| `has(field)` | Returns true if the field exists and is not null | `has(payload.pull_request)` |

### Data Paths

Expressions can reference data from three namespaces:

| Prefix | Description | Example |
|--------|-------------|---------|
| `attributes.*` | Pub/Sub message attributes | `attributes.gh_event`, `attributes.action` |
| `payload.*` | JSON payload of the webhook event | `payload.repository.visibility`, `payload.pull_request.node_id` |
| `meta.*` | Event metadata | `meta.message_id` |

### Example Expressions

```
# Match opened PRs or issues on public repositories
(attributes.gh_event = "pull_request" OR attributes.gh_event = "issues") AND
attributes.action = "opened" AND
payload.repository.visibility = "public"

# Match any push event
attributes.gh_event = "push"

# Match events that have a pull_request field
has(payload.pull_request) AND attributes.action = "opened"
```

---

## Steps (`then`)

Each step in the `then` list defines an action to execute. Steps are executed **in order**, and each step can reference outputs from previous steps.

| Field | Required | Description |
|-------|----------|-------------|
| `name` | Yes | Unique name for the step (used for output references and logging) |
| `type` | Yes | Step type — currently only `github_graphql` |
| `config` | Yes | Step-type-specific configuration |

---

## `github_graphql` Step Config

Executes a GitHub GraphQL mutation or query.

| Field | Required | Description |
|-------|----------|-------------|
| `document` | Yes | GraphQL mutation or query string |
| `variables` | Yes | Map of variable names to values (supports interpolation) |
| `outputs` | No | Map of output names to dot-paths in the GraphQL response |

### `document`

The full GraphQL document to execute. Use YAML block scalars (`|`) for multi-line documents:

```yaml
document: |
  mutation AddItemToProject($projectId: ID!, $contentId: ID!) {
    addProjectV2ItemById(input: { projectId: $projectId, contentId: $contentId }) {
      item {
        id
      }
    }
  }
```

### `variables`

A map of GraphQL variable names to values. Values support variable interpolation (see below).

```yaml
variables:
  projectId: ${constants.projects.public.project_id}
  contentId: ${payload.pull_request.node_id}
```

### `outputs`

A map of output names to dot-separated paths into the GraphQL JSON response. Outputs can be referenced by subsequent steps.

```yaml
outputs:
  itemId: data.addProjectV2ItemById.item.id
```

The path `data.addProjectV2ItemById.item.id` traverses the JSON response to extract the value.

---

## Variable Interpolation

Step variables support `${...}` interpolation syntax. The following reference types are available:

| Pattern | Description | Example |
|---------|-------------|---------|
| `${constants.path}` | Value from the constants map | `${constants.projects.public.project_id}` |
| `${payload.path}` | Value from the webhook event JSON payload | `${payload.pull_request.node_id}` |
| `${attributes.name}` | Value from the Pub/Sub message attributes | `${attributes.gh_event}` |
| `${meta.name}` | Value from event metadata | `${meta.message_id}` |
| `${steps.step_name.outputs.key}` | Output from a previous step | `${steps.add_to_project.outputs.itemId}` |
| `${coalesce(path1, path2)}` | First non-null value from the listed paths | `${coalesce(payload.pull_request.node_id, payload.issue.node_id)}` |

### Coalesce

The `coalesce()` function returns the first non-null value from its arguments. This is useful when a field may exist in different locations depending on the event type:

```yaml
contentId: ${coalesce(payload.pull_request.node_id, payload.issue.node_id)}
```

### String Interpolation

If a variable value is entirely a single `${...}` reference, the raw resolved value is returned (preserving its type). If the value contains a mix of literal text and references, all references are interpolated as strings.
