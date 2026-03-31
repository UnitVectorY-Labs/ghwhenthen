---
layout: default
title: Examples
nav_order: 4
---

# Examples

## Public PR/Issue Routing to GitHub Projects

This example demonstrates the primary use case for ghwhenthen: automatically adding newly opened pull requests and issues to a GitHub Project V2 board and setting their status.

### Full Configuration

The complete example is available at [`examples/config.yaml`](https://github.com/UnitVectorY-Labs/ghwhenthen/blob/main/examples/config.yaml) in the repository.

```yaml
pubsub:
  project_id: my-gcp-project
  subscription_id: GitHub-PR-Issue-Opened

github:
  token_env_var: GITHUB_TOKEN
  graphql_endpoint: https://api.github.com/graphql

behavior:
  on_failure: nack

constants:
  projects:
    public:
      project_id: PVT_kwDO_PUBLIC
      status_field_id: PVTSSF_PUBLIC_STATUS
      todo_option_id: f75ad846
    private:
      project_id: PVT_kwDO_PRIVATE
      status_field_id: PVTSSF_PRIVATE_STATUS
      todo_option_id: f75ad846

rules:
  - name: public-opened-item-to-public-project
    enabled: true
    when: >
      (attributes.gh_event = "pull_request" OR attributes.gh_event = "issues") AND
      attributes.action = "opened" AND
      (payload.repository.visibility = "public" OR payload.repository.private = false)
    then:
      - name: add_to_project
        type: github_graphql
        config:
          document: |
            mutation AddItemToProject($projectId: ID!, $contentId: ID!) {
              addProjectV2ItemById(input: { projectId: $projectId, contentId: $contentId }) {
                item {
                  id
                }
              }
            }
          variables:
            projectId: ${constants.projects.public.project_id}
            contentId: ${coalesce(payload.pull_request.node_id, payload.issue.node_id)}
          outputs:
            itemId: data.addProjectV2ItemById.item.id

      - name: set_status_todo
        type: github_graphql
        config:
          document: |
            mutation SetProjectFieldValue(
              $projectId: ID!,
              $itemId: ID!,
              $fieldId: ID!,
              $optionId: String!
            ) {
              updateProjectV2ItemFieldValue(input: {
                projectId: $projectId
                itemId: $itemId
                fieldId: $fieldId
                value: { singleSelectOptionId: $optionId }
              }) {
                projectV2Item {
                  id
                }
              }
            }
          variables:
            projectId: ${constants.projects.public.project_id}
            itemId: ${steps.add_to_project.outputs.itemId}
            fieldId: ${constants.projects.public.status_field_id}
            optionId: ${constants.projects.public.todo_option_id}
          outputs:
            updatedItemId: data.updateProjectV2ItemFieldValue.projectV2Item.id
```

### How It Works

#### Step 1: Rule Matching

When a Pub/Sub message arrives, the rule engine evaluates the `when` expression:

```
(attributes.gh_event = "pull_request" OR attributes.gh_event = "issues") AND
attributes.action = "opened" AND
(payload.repository.visibility = "public" OR payload.repository.private = false)
```

This matches when:
- The event is a pull request **or** an issue
- The action is `"opened"` (newly created)
- The repository is public (checked two ways for compatibility)

#### Step 2: Add to Project

The first `then` step calls the `addProjectV2ItemById` GraphQL mutation:

- **`projectId`** is resolved from `${constants.projects.public.project_id}` — the constant defined at the top of the config
- **`contentId`** uses `${coalesce(payload.pull_request.node_id, payload.issue.node_id)}` — this picks the PR's node ID if available, otherwise falls back to the issue's node ID
- The **output** `itemId` captures the newly created project item's ID from the GraphQL response at path `data.addProjectV2ItemById.item.id`

#### Step 3: Set Status

The second step uses the output from step 1 to set the project item's status:

- **`itemId`** is resolved from `${steps.add_to_project.outputs.itemId}` — referencing the output of the previous step
- **`fieldId`** and **`optionId`** come from constants, pointing to the "Status" field and the "Todo" option

This two-step workflow demonstrates **output chaining**: the result of one GraphQL call becomes the input to the next.

### Public vs. Private Routing

The example config includes two rules with identical step structures but different constants — one for public repositories routing to a public project board, and one for private repositories routing to a private project board. The `when` expression on each rule directs events to the appropriate project.

## Finding Your GitHub Project IDs

To use ghwhenthen with GitHub Projects V2, you need several node IDs. You can find these using GitHub's GraphQL Explorer or the `gh` CLI:

```bash
# Find your project's node ID
gh api graphql -f query='
  query {
    user(login: "YOUR_USERNAME") {
      projectV2(number: PROJECT_NUMBER) {
        id
        fields(first: 20) {
          nodes {
            ... on ProjectV2SingleSelectField {
              id
              name
              options {
                id
                name
              }
            }
          }
        }
      }
    }
  }
'
```

This returns the `project_id`, `status_field_id`, and `todo_option_id` values needed for your constants.
