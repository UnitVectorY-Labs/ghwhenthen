---
layout: default
title: Home
nav_order: 1
---

# ghwhenthen

A lightweight, configuration-driven service that consumes GitHub webhook events from Google Cloud Pub/Sub, evaluates declarative rules against message attributes and payloads, and executes ordered GitHub GraphQL actions.

## What is ghwhenthen?

**ghwhenthen** bridges GitHub webhooks and GitHub's GraphQL API through Google Cloud Pub/Sub. You define declarative "when/then" rules in YAML: *when* a webhook event matches your conditions, *then* execute a sequence of GitHub GraphQL mutations.

This enables powerful automation workflows — such as automatically adding new pull requests to a GitHub Project board and setting their status — without writing any code.

## Key Features

- **Pub/Sub Consumption** — Subscribes to a Google Cloud Pub/Sub topic that receives GitHub webhook events, with automatic gzip decompression support.
- **Declarative Rule Engine** — Define rules using a simple expression language with `AND`, `OR`, `=`, `!=`, `has()`, and parenthesized grouping.
- **GitHub GraphQL Actions** — Execute ordered sequences of GraphQL mutations/queries with full variable interpolation and step-to-step output chaining.
- **Configuration-Driven** — All behavior is defined in a single YAML file. No code changes needed to add new automation rules.
- **Kubernetes Ready** — Ships as a minimal Docker container with health endpoints, structured JSON logging, and graceful shutdown on SIGINT/SIGTERM.
- **Constants & Reusability** — Define reusable values (project IDs, field IDs) in a constants block and reference them across rules.

## Getting Started

1. [Configure](CONFIGURATION.md) your rules in a YAML file
2. [Set up](COMMAND.md) the command-line flags and environment variables
3. Deploy to Kubernetes or run locally
4. Check out the [Examples](EXAMPLES.md) for real-world use cases

## How It Works

```
GitHub Webhook → Pub/Sub → ghwhenthen → GitHub GraphQL API
```

1. GitHub sends webhook events to a Pub/Sub topic (via a push subscription or relay)
2. **ghwhenthen** pulls messages from a Pub/Sub subscription
3. Each message's attributes and JSON payload are evaluated against your rules
4. When a rule's `when` expression matches, its `then` steps execute in order
5. Steps can chain outputs — use the result of one GraphQL call as input to the next
