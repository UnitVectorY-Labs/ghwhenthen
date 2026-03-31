---
layout: default
title: Command Line
nav_order: 2
---

# Command Line

## Flags and Environment Variables

ghwhenthen supports the following command-line flags. Each flag has a corresponding environment variable. The precedence order is:

**Flag > Environment Variable > Default Value**

| Flag | Environment Variable | Description | Default |
|------|---------------------|-------------|---------|
| `--config` | `GHWHENTHEN_CONFIG` | Path to YAML config file | *(required)* |
| `--port` | `GHWHENTHEN_PORT` | Health endpoint port | `8080` |

### `--config` / `GHWHENTHEN_CONFIG`

Path to the YAML configuration file. This is required — the service will exit with an error if not provided.

```bash
ghwhenthen --config /etc/ghwhenthen/config.yaml
```

Or via environment variable:

```bash
export GHWHENTHEN_CONFIG=/etc/ghwhenthen/config.yaml
ghwhenthen
```

### `--port` / `GHWHENTHEN_PORT`

The port for the HTTP health endpoint. The health server exposes a `/` endpoint that returns the service's liveness status. Defaults to `8080`.

```bash
ghwhenthen --config config.yaml --port 9090
```

## GitHub Token

The GitHub Personal Access Token (PAT) is **not** configured via a flag. Instead, the YAML configuration specifies which environment variable contains the token using `github.token_env_var`:

```yaml
github:
  token_env_var: GITHUB_TOKEN
```

The service reads the token from the named environment variable at startup. If the variable is empty or unset, the service exits with an error.

```bash
export GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx
```

The token needs sufficient permissions for the GraphQL mutations defined in your rules (e.g., `project` scope for GitHub Projects V2 operations).

## Google Cloud Credentials

ghwhenthen uses the standard Google Cloud SDK credential resolution. No flags or environment variables are needed if running on GKE with Workload Identity. For other environments, configure credentials using one of:

- **Workload Identity** (recommended for GKE) — automatically provided by the environment
- **`GOOGLE_APPLICATION_CREDENTIALS`** — path to a service account JSON key file
- **Application Default Credentials** — via `gcloud auth application-default login`

The service account or identity must have the `roles/pubsub.subscriber` role (or equivalent permissions) on the configured Pub/Sub subscription.

## Examples

### Running locally

```bash
export GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx
export GOOGLE_APPLICATION_CREDENTIALS=/path/to/sa-key.json
ghwhenthen --config config.yaml
```

### Kubernetes deployment

```yaml
containers:
  - name: ghwhenthen
    image: ghcr.io/unitvectory-labs/ghwhenthen:latest
    args:
      - --config
      - /etc/ghwhenthen/config.yaml
    env:
      - name: GITHUB_TOKEN
        valueFrom:
          secretKeyRef:
            name: github-token
            key: token
    ports:
      - containerPort: 8080
    livenessProbe:
      httpGet:
        path: /
        port: 8080
```
