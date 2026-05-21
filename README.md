# Lawmatics CLI

**Every Lawmatics resource the API exposes, plus offline full-text search, custom-field reports, and intake analytics no Lawmatics tool has.**

Lawmatics-pp-cli mirrors your firm's CRM into a local SQLite database so every contact, matter, note, and interaction is searchable in milliseconds. It adds the bulk operations and analytics the Lawmatics UI doesn't: cross-matter overdue task lists, stage bottleneck reports, referral-source revenue, and ad-hoc custom-field pivots straight to CSV.

Printed by [@gregvanhorn](https://github.com/gregvanhorn) (Greg Van Horn).

## Install

The recommended path installs both the `lawmatics-pp-cli` binary and the `pp-lawmatics` agent skill (Claude Code, Codex, Cursor, Gemini CLI, GitHub Copilot, and other agents supported by the upstream [`skills`](https://github.com/vercel-labs/skills) CLI) in one shot:

```bash
npx -y @mvanhorn/printing-press install lawmatics
```

For CLI only (no skill):

```bash
npx -y @mvanhorn/printing-press install lawmatics --cli-only
```

For skill only — installs the skill into the same agents as the default command above, but skips the CLI binary (use this to update or reinstall just the skill):

```bash
npx -y @mvanhorn/printing-press install lawmatics --skill-only
```

To constrain the skill install to one or more specific agents (repeatable — agent names match the [`skills`](https://github.com/vercel-labs/skills) CLI):

```bash
npx -y @mvanhorn/printing-press install lawmatics --agent claude-code
npx -y @mvanhorn/printing-press install lawmatics --agent claude-code --agent codex
```

### Without Node

The generated install path is category-agnostic until this CLI is published. If `npx` is not available before publish, install Node or use the category-specific Go fallback from the public-library entry after publish.

### Pre-built binary

Download a pre-built binary for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/lawmatics-current). On macOS, clear the Gatekeeper quarantine: `xattr -d com.apple.quarantine <binary>`. On Unix, mark it executable: `chmod +x <binary>`.

<!-- pp-hermes-install-anchor -->
## Install for Hermes

From the Hermes CLI:

```bash
hermes skills install mvanhorn/printing-press-library/cli-skills/pp-lawmatics --force
```

Inside a Hermes chat session:

```bash
/skills install mvanhorn/printing-press-library/cli-skills/pp-lawmatics --force
```

## Install for OpenClaw

Tell your OpenClaw agent (copy this):

```
Install the pp-lawmatics skill from https://github.com/mvanhorn/printing-press-library/tree/main/cli-skills/pp-lawmatics. The skill defines how its required CLI can be installed.
```

## Use with Claude Desktop

This CLI ships an [MCPB](https://github.com/modelcontextprotocol/mcpb) bundle — Claude Desktop's standard format for one-click MCP extension installs (no JSON config required).

The bundle reuses your local OAuth tokens — authenticate first if you haven't:

```bash
lawmatics-pp-cli auth login
```

To install:

1. Download the `.mcpb` for your platform from the [latest release](https://github.com/mvanhorn/printing-press-library/releases/tag/lawmatics-current).
2. Double-click the `.mcpb` file. Claude Desktop opens and walks you through the install.
3. Fill in `LAWMATICS_ACCESS_TOKEN` when Claude Desktop prompts you.

Requires Claude Desktop 1.0.0 or later. Pre-built bundles ship for macOS Apple Silicon (`darwin-arm64`) and Windows (`amd64`, `arm64`); for other platforms, use the manual config below.

<details>
<summary>Manual JSON config (advanced)</summary>

If you can't use the MCPB bundle (older Claude Desktop, unsupported platform), install the MCP binary and configure it manually.


Install the MCP binary from this CLI's published public-library entry or pre-built release.

Add to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "lawmatics": {
      "command": "lawmatics-pp-mcp",
      "env": {
        "LAWMATICS_ACCESS_TOKEN": "<your-key>"
      }
    }
  }
}
```

</details>

## Authentication

Lawmatics uses OAuth 2.0. Run `lawmatics-pp-cli auth login` to open the browser, approve, and the CLI catches the redirect at http://localhost:8765/callback and stores the token in the macOS Keychain. Static bearer tokens are also supported via `LAWMATICS_ACCESS_TOKEN`.

## Quick Start

```bash
# OAuth flow into your Lawmatics firm
lawmatics-pp-cli auth login


# Mirror contacts, matters, notes, tasks into local SQLite
lawmatics-pp-cli sync


# Sub-100ms offline FTS across every synced entity
lawmatics-pp-cli search "slip and fall" --json


# Standup view of past-due tasks across every matter
lawmatics-pp-cli overdue --by assignee --json


# Pivot custom fields the web UI can't
lawmatics-pp-cli cf report --fields "Case Value,Referral" --csv

```

## Unique Features

These capabilities aren't available in any other tool for this API.

### Local state that compounds
- **`search`** — Sub-100ms offline full-text search across contacts, matters, notes, comments, and interactions including custom fields.

  _Reach for this instead of the API when an agent needs cross-entity search by free text._

  ```bash
  lawmatics-pp-cli search "slip and fall" --json
  ```
- **`bottleneck`** — Median matter dwell-time per intake/pipeline stage, surfacing where deals get stuck.

  _Use to answer 'where are matters dying?' without exporting CSVs._

  ```bash
  lawmatics-pp-cli bottleneck --pipeline intake --json
  ```
- **`overdue`** — Every past-due task across every matter, grouped by assignee and sorted by lateness.

  _Standup-ready workload view the Lawmatics UI cannot produce in one click._

  ```bash
  lawmatics-pp-cli overdue --by assignee --json
  ```
- **`since`** — Everything created or updated in the last N hours/days across all entities.

  _Intake-team standup view: what did we touch yesterday?_

  ```bash
  lawmatics-pp-cli since 24h --json
  ```
- **`cf report`** — Join contacts and matters with arbitrary custom fields and pivot to CSV/JSON.

  _Use when the firm needs ad-hoc analytics the web UI's report builder cannot do._

  ```bash
  lawmatics-pp-cli cf report --fields "Case Value,Referral,Stage" --where stage=signed --csv
  ```
- **`velocity`** — Rolling conversion rate (prospect to signed matter) per referral source with sparklines.

  _Marketing ROI question the API answers only with raw lists._

  ```bash
  lawmatics-pp-cli velocity --by source --window 90d --json
  ```
- **`intake stale`** — Prospects with zero interactions in N days.

  _Surfaces leads that are about to die._

  ```bash
  lawmatics-pp-cli intake stale --days 7 --json
  ```
- **`who-touched`** — Unified chronological feed of every interaction, note, task, and email across all matters for a contact.

  _Use before a re-engagement call or status meeting._

  ```bash
  lawmatics-pp-cli who-touched jane.smith@example.com --json
  ```
- **`revenue`** — Sums invoiced and paid amounts per referral source or practice area.

  _Real marketing-channel ROI in one call._

  ```bash
  lawmatics-pp-cli revenue --by source --window ytd --json
  ```
- **`load`** — Open matters, overdue tasks, and billable hours MTD per attorney.

  _Workload balancing in one command._

  ```bash
  lawmatics-pp-cli load --by attorney --json
  ```
- **`explain`** — Markdown brief summarizing a matter from notes, interactions, and tasks.

  _Use before a status call to brief in 5 seconds._

  ```bash
  lawmatics-pp-cli explain MAT-1234 --json
  ```
- **`pipeline drift`** — Matters that skipped a stage or moved backward through the pipeline.

  _Use to find broken intake processes before clients notice._

  ```bash
  lawmatics-pp-cli pipeline drift --json
  ```

### Bulk operations
- **`bulk reassign`** — Reassign every task matching a filter from one user to another in one command.

  _Use during attorney transitions or PTO coverage instead of clicking through hundreds of tasks._

  ```bash
  lawmatics-pp-cli bulk reassign --from alice@firm.com --to bob@firm.com --filter matter.status=open --dry-run
  ```

### Agent-native plumbing
- **`doctor`** — Health check: orphan tasks, contacts with no email, matters in dead stages, custom fields never used, dormant users.

  _Run quarterly to keep the firm's CRM data clean._

  ```bash
  lawmatics-pp-cli doctor --json
  ```
- **`watch`** — Local daemon that polls and POSTs entity diffs to a URL of your choice.

  _Use when integrating Lawmatics with internal automation without paying for native webhooks._

  ```bash
  lawmatics-pp-cli watch contacts --webhook https://hooks.firm.com/lm --since 5m
  ```

## Usage

Run `lawmatics-pp-cli --help` for the full command reference and flag list.

## Commands

### addresses

Operations on addresses

- **`lawmatics-pp-cli addresses create`** - Create an address
- **`lawmatics-pp-cli addresses delete`** - Delete an address
- **`lawmatics-pp-cli addresses get`** - Get an address
- **`lawmatics-pp-cli addresses list`** - List addresses
- **`lawmatics-pp-cli addresses update`** - Update an address

### comments

Operations on comments

- **`lawmatics-pp-cli comments create`** - Create a comment
- **`lawmatics-pp-cli comments delete`** - Delete a comment
- **`lawmatics-pp-cli comments get`** - Get a comment
- **`lawmatics-pp-cli comments list`** - List comments
- **`lawmatics-pp-cli comments update`** - Update a comment

### companies

Operations on companies

- **`lawmatics-pp-cli companies create`** - Create a new company
- **`lawmatics-pp-cli companies delete`** - Delete a company
- **`lawmatics-pp-cli companies find-by-email`** - Find a company by email
- **`lawmatics-pp-cli companies find-by-name`** - Find a company by name
- **`lawmatics-pp-cli companies find-by-phone`** - Find a company by phone
- **`lawmatics-pp-cli companies get`** - Get a company by ID
- **`lawmatics-pp-cli companies list`** - List all companies
- **`lawmatics-pp-cli companies update`** - Update a company

### contacts

Operations on contacts

- **`lawmatics-pp-cli contacts create`** - Create a new contact
- **`lawmatics-pp-cli contacts delete`** - Delete a contact
- **`lawmatics-pp-cli contacts find-by-email`** - Find a contact by email
- **`lawmatics-pp-cli contacts find-by-name`** - Find a contact by name
- **`lawmatics-pp-cli contacts find-by-phone`** - Find a contact by phone
- **`lawmatics-pp-cli contacts get`** - Get a contact by ID
- **`lawmatics-pp-cli contacts list`** - List all contacts
- **`lawmatics-pp-cli contacts update`** - Update a contact

### custom_contact_types

Operations on custom contact types

- **`lawmatics-pp-cli custom_contact_types create`** - Create a custom contact type
- **`lawmatics-pp-cli custom_contact_types delete`** - Delete a custom contact type
- **`lawmatics-pp-cli custom_contact_types get`** - Get a custom contact type
- **`lawmatics-pp-cli custom_contact_types list`** - List custom contact types
- **`lawmatics-pp-cli custom_contact_types update`** - Update a custom contact type

### custom_fields

Operations on custom fields

- **`lawmatics-pp-cli custom_fields create`** - Create a custom field
- **`lawmatics-pp-cli custom_fields delete`** - Delete a custom field
- **`lawmatics-pp-cli custom_fields get`** - Get a custom field
- **`lawmatics-pp-cli custom_fields list`** - List custom fields
- **`lawmatics-pp-cli custom_fields update`** - Update a custom field

### email_addresses

Operations on email addresses

- **`lawmatics-pp-cli email_addresses create`** - Create an email address
- **`lawmatics-pp-cli email_addresses delete`** - Delete an email address
- **`lawmatics-pp-cli email_addresses get`** - Get an email address
- **`lawmatics-pp-cli email_addresses list`** - List email addresses
- **`lawmatics-pp-cli email_addresses update`** - Update an email address

### email_campaigns

Operations on email campaigns

- **`lawmatics-pp-cli email_campaigns get`** - Get an email campaign
- **`lawmatics-pp-cli email_campaigns list`** - List email campaigns
- **`lawmatics-pp-cli email_campaigns stats`** - Get email campaign stats

### event_types

Operations on event types

- **`lawmatics-pp-cli event_types create`** - Create an event type
- **`lawmatics-pp-cli event_types delete`** - Delete an event type
- **`lawmatics-pp-cli event_types get`** - Get an event type
- **`lawmatics-pp-cli event_types list`** - List event types
- **`lawmatics-pp-cli event_types update`** - Update an event type

### events

Operations on events

- **`lawmatics-pp-cli events create`** - Create an event
- **`lawmatics-pp-cli events delete`** - Delete an event
- **`lawmatics-pp-cli events get`** - Get an event
- **`lawmatics-pp-cli events list`** - List events

### expenses

Operations on expenses

- **`lawmatics-pp-cli expenses create`** - Create an expense
- **`lawmatics-pp-cli expenses delete`** - Delete an expense
- **`lawmatics-pp-cli expenses get`** - Get an expense
- **`lawmatics-pp-cli expenses list`** - List expenses
- **`lawmatics-pp-cli expenses update`** - Update an expense

### files

Operations on files

- **`lawmatics-pp-cli files delete`** - Delete a file
- **`lawmatics-pp-cli files download`** - Download a file
- **`lawmatics-pp-cli files get`** - Get a file
- **`lawmatics-pp-cli files list`** - List files
- **`lawmatics-pp-cli files update`** - Update a file
- **`lawmatics-pp-cli files upload`** - Upload a file

### folders

Operations on folders

- **`lawmatics-pp-cli folders create`** - Create a folder
- **`lawmatics-pp-cli folders delete`** - Delete a folder
- **`lawmatics-pp-cli folders get`** - Get a folder
- **`lawmatics-pp-cli folders list`** - List folders
- **`lawmatics-pp-cli folders update`** - Update a folder

### interactions

Operations on interactions

- **`lawmatics-pp-cli interactions create`** - Create an interaction
- **`lawmatics-pp-cli interactions delete`** - Delete an interaction
- **`lawmatics-pp-cli interactions get`** - Get an interaction
- **`lawmatics-pp-cli interactions list`** - List interactions
- **`lawmatics-pp-cli interactions update`** - Update an interaction

### invoices

Operations on invoices

- **`lawmatics-pp-cli invoices get`** - Get an invoice
- **`lawmatics-pp-cli invoices list`** - List invoices

### locations

Operations on locations

- **`lawmatics-pp-cli locations`** - List locations

### matter_sub_statuses

Operations on matter sub statuses

- **`lawmatics-pp-cli matter_sub_statuses create`** - Create a matter sub status
- **`lawmatics-pp-cli matter_sub_statuses delete`** - Delete a matter sub status
- **`lawmatics-pp-cli matter_sub_statuses update`** - Update a matter sub status

### matters

Operations on matters

- **`lawmatics-pp-cli matters create`** - Create a new matter
- **`lawmatics-pp-cli matters delete`** - Delete a matter
- **`lawmatics-pp-cli matters find-by-email`** - Find a matter by email
- **`lawmatics-pp-cli matters get`** - Get a matter by ID
- **`lawmatics-pp-cli matters list`** - List all matters
- **`lawmatics-pp-cli matters update`** - Update a matter

### notes

Operations on notes

- **`lawmatics-pp-cli notes create`** - Create a note
- **`lawmatics-pp-cli notes delete`** - Delete a note
- **`lawmatics-pp-cli notes get`** - Get a note
- **`lawmatics-pp-cli notes list`** - List notes
- **`lawmatics-pp-cli notes update`** - Update a note

### phone_numbers

Operations on phone numbers

- **`lawmatics-pp-cli phone_numbers create`** - Create a phone number
- **`lawmatics-pp-cli phone_numbers delete`** - Delete a phone number
- **`lawmatics-pp-cli phone_numbers get`** - Get a phone number
- **`lawmatics-pp-cli phone_numbers list`** - List phone numbers
- **`lawmatics-pp-cli phone_numbers update`** - Update a phone number

### pipelines

Operations on pipelines

- **`lawmatics-pp-cli pipelines get`** - Get a pipeline by ID
- **`lawmatics-pp-cli pipelines list`** - List pipelines

### practice_areas

Operations on practice areas

- **`lawmatics-pp-cli practice_areas create`** - Create a practice area
- **`lawmatics-pp-cli practice_areas delete`** - Delete a practice area
- **`lawmatics-pp-cli practice_areas get`** - Get a practice area
- **`lawmatics-pp-cli practice_areas list`** - List practice areas
- **`lawmatics-pp-cli practice_areas update`** - Update a practice area

### prospects

Operations on prospects

- **`lawmatics-pp-cli prospects create`** - Create a prospect
- **`lawmatics-pp-cli prospects delete`** - Delete a prospect
- **`lawmatics-pp-cli prospects get`** - Get a prospect
- **`lawmatics-pp-cli prospects list`** - List prospects
- **`lawmatics-pp-cli prospects update`** - Update a prospect

### relationship_types

Operations on relationship types

- **`lawmatics-pp-cli relationship_types create`** - Create a relationship type
- **`lawmatics-pp-cli relationship_types get`** - Get a relationship type
- **`lawmatics-pp-cli relationship_types list`** - List relationship types
- **`lawmatics-pp-cli relationship_types update`** - Update a relationship type

### relationships

Operations on relationships

- **`lawmatics-pp-cli relationships create`** - Create a relationship
- **`lawmatics-pp-cli relationships delete`** - Delete a relationship
- **`lawmatics-pp-cli relationships get`** - Get a relationship
- **`lawmatics-pp-cli relationships list`** - List relationships
- **`lawmatics-pp-cli relationships update`** - Update a relationship

### stages

Operations on stages

- **`lawmatics-pp-cli stages get`** - Get a stage
- **`lawmatics-pp-cli stages list`** - List stages

### subtasks

Operations on subtasks

- **`lawmatics-pp-cli subtasks create`** - Create a subtask
- **`lawmatics-pp-cli subtasks delete`** - Delete a subtask
- **`lawmatics-pp-cli subtasks get`** - Get a subtask
- **`lawmatics-pp-cli subtasks list`** - List subtasks
- **`lawmatics-pp-cli subtasks update`** - Update a subtask

### tags

Operations on tags

- **`lawmatics-pp-cli tags create`** - Create a tag
- **`lawmatics-pp-cli tags delete`** - Delete a tag
- **`lawmatics-pp-cli tags get`** - Get a tag
- **`lawmatics-pp-cli tags list`** - List tags
- **`lawmatics-pp-cli tags update`** - Update a tag

### task_statuses

Operations on task statuses

- **`lawmatics-pp-cli task_statuses create`** - Create a task status
- **`lawmatics-pp-cli task_statuses delete`** - Delete a task status
- **`lawmatics-pp-cli task_statuses get`** - Get a task status
- **`lawmatics-pp-cli task_statuses list`** - List task statuses
- **`lawmatics-pp-cli task_statuses update`** - Update a task status

### tasks

Operations on tasks

- **`lawmatics-pp-cli tasks create`** - Create a task
- **`lawmatics-pp-cli tasks delete`** - Delete a task
- **`lawmatics-pp-cli tasks get`** - Get a task
- **`lawmatics-pp-cli tasks list`** - List tasks
- **`lawmatics-pp-cli tasks update`** - Update a task

### time_entries

Operations on time entries

- **`lawmatics-pp-cli time_entries create`** - Create a time entry
- **`lawmatics-pp-cli time_entries delete`** - Delete a time entry
- **`lawmatics-pp-cli time_entries get`** - Get a time entry
- **`lawmatics-pp-cli time_entries list`** - List time entries
- **`lawmatics-pp-cli time_entries update`** - Update a time entry

### transactions

Operations on transactions

- **`lawmatics-pp-cli transactions create`** - Create a transaction
- **`lawmatics-pp-cli transactions get`** - Get a transaction
- **`lawmatics-pp-cli transactions list`** - List transactions

### users

Operations on users

- **`lawmatics-pp-cli users create`** - Create a user
- **`lawmatics-pp-cli users delete`** - Delete a user
- **`lawmatics-pp-cli users get`** - Get a user
- **`lawmatics-pp-cli users list`** - List users
- **`lawmatics-pp-cli users update`** - Update a user


## Output Formats

```bash
# Human-readable table (default in terminal, JSON when piped)
lawmatics-pp-cli addresses list

# JSON for scripting and agents
lawmatics-pp-cli addresses list --json

# Filter to specific fields
lawmatics-pp-cli addresses list --json --select id,name,status

# Dry run — show the request without sending
lawmatics-pp-cli addresses list --dry-run

# Agent mode — JSON + compact + no prompts in one flag
lawmatics-pp-cli addresses list --agent
```

## Agent Usage

This CLI is designed for AI agent consumption:

- **Non-interactive** - never prompts, every input is a flag
- **Pipeable** - `--json` output to stdout, errors to stderr
- **Filterable** - `--select id,name` returns only fields you need
- **Previewable** - `--dry-run` shows the request without sending
- **Explicit retries** - add `--idempotent` to create retries and `--ignore-missing` to delete retries when a no-op success is acceptable
- **Confirmable** - `--yes` for explicit confirmation of destructive actions
- **Piped input** - write commands can accept structured input when their help lists `--stdin`
- **Offline-friendly** - sync/search commands can use the local SQLite store when available
- **Agent-safe by default** - no colors or formatting unless `--human-friendly` is set

Exit codes: `0` success, `2` usage error, `3` not found, `4` auth error, `5` API error, `7` rate limited, `10` config error.

## Health Check

```bash
lawmatics-pp-cli doctor
```

Verifies configuration, credentials, and connectivity to the API.

## Configuration

Config file: `~/.config/lawmatics-pp-cli/config.toml`

Static request headers can be configured under `headers`; per-command header overrides take precedence.

Environment variables:

| Name | Kind | Required | Description |
| --- | --- | --- | --- |
| `LAWMATICS_ACCESS_TOKEN` | per_call | Yes | Set to your API credential. |

## Troubleshooting
**Authentication errors (exit code 4)**
- Run `lawmatics-pp-cli doctor` to check credentials
- Verify the environment variable is set: `echo $LAWMATICS_ACCESS_TOKEN`
**Not found errors (exit code 3)**
- Check the resource ID is correct
- Run the `list` command to see available items

### API-specific

- **auth login fails with 'redirect_uri mismatch'** — Set the Lawmatics app's callback URL to http://localhost:8765/callback or pass --port to match
- **429 rate limit hit during sync** — Lawmatics caps at 1000 req/min per firm; run sync with --rate 800 to stay below
- **Access token expired** — Run `lawmatics-pp-cli auth refresh`; OAuth refresh tokens are stored automatically after login

---

## Sources & Inspiration

This CLI was built by studying these projects and resources:

- [**mjquinlan2000/lawmatics-mcp**](https://github.com/mjquinlan2000/lawmatics-mcp) — TypeScript
- [**HarderBetterFasterStronger/lawmatics-mcp**](https://github.com/HarderBetterFasterStronger/lawmatics-mcp) — TypeScript
- [**martyvasquez/lawmatics-mcp-v1.0**](https://github.com/martyvasquez/lawmatics-mcp-v1.0) — Python
- [**PipedreamHQ/pipedream (lawmatics)**](https://github.com/PipedreamHQ/pipedream/tree/master/components/lawmatics) — JavaScript
- [**bhubbard/wp-lawmatics-api**](https://github.com/bhubbard/wp-lawmatics-api) — PHP

Generated by [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
