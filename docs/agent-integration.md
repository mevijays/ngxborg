# Agent integration

ngxborg exposes a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server at the `/mcp` endpoint, allowing AI agents and LLM-powered tools to manage your Borg backup infrastructure through structured, typed tool calls.

## Overview

The MCP server is embedded in the ngxborg binary — no separate process, no extra installation. It runs on the same port as the web UI and CLI, accessible at `/mcp` via HTTP POST.

```
http://your-server:8443/mcp
```

The MCP endpoint is **anonymous** — it does not require PAM authentication. This is intentional: agents connecting from external systems (GitHub Actions, CI/CD pipelines, local LLM tools) cannot authenticate via PAM, and the endpoint is only reachable on your internal network or behind a reverse proxy with its own access control.

## Transport

ngxborg uses the **Streamable HTTP** transport (not stdio). Clients connect via HTTP POST with JSON-RPC 2.0 payloads. The server returns a `Mcp-Session-Id` header on the `initialize` call, which must be included in all subsequent requests.

### Protocol flow

1. **Initialize** — send a JSON-RPC `initialize` request with your client info.
2. **List tools** — use the returned session ID to call `tools/list`.
3. **Call tools** — invoke individual tools with `tools/call`.

### Example: Initialize

```bash
curl -s -X POST http://172.16.1.107:8443/mcp \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2025-03-26",
      "capabilities": {},
      "clientInfo": {
        "name": "my-agent",
        "version": "1.0.0"
      }
    }
  }'
```

Response:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2025-03-26",
    "serverInfo": {
      "name": "ngxborg",
      "version": "1.0.0"
    },
    "capabilities": {
      "tools": {
        "listChanged": true
      }
    }
  }
}
```

The response includes a `Mcp-Session-Id` header — capture this for all subsequent calls.

### Example: List tools

```bash
curl -s -X POST http://172.16.1.107:8443/mcp \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: mcp-session-<uuid>" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list",
    "params": {}
  }'
```

### Example: Call a tool

```bash
curl -s -X POST http://172.16.1.107:8443/mcp \
  -H "Content-Type: application/json" \
  -H "Mcp-Session-Id: mcp-session-<uuid>" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "ngxborg_user_create",
      "arguments": {
        "username": "bob",
        "admin": false
      }
    }
  }'
```

## Available tools

ngxborg exposes **17 tools** organized into three categories.

### User management (8 tools)

| Tool | Description | Required arguments | Optional arguments |
|------|-------------|-------------------|-------------------|
| `ngxborg_user_create` | Create a new tenant or admin user | `username` | `admin` (boolean, default: false) |
| `ngxborg_user_delete` | Delete a user and their account | `username` | — |
| `ngxborg_user_list` | List all ngxborg users | — | — |
| `ngxborg_user_disable` | Disable a user (locks SSH keys and web UI) | `username` | — |
| `ngxborg_user_enable` | Enable a disabled user | `username` | — |
| `ngxborg_user_key_add` | Add an SSH key for a user | `username`, `key`, `label` | — |
| `ngxborg_user_key_list` | List SSH keys for a user | `username` | — |
| `ngxborg_user_key_remove` | Remove an SSH key from a user | `username`, `key` | — |

### Repository management (6 tools)

| Tool | Description | Required arguments | Optional arguments |
|------|-------------|-------------------|-------------------|
| `ngxborg_repo_create` | Create a new backup repository | `tenant`, `name` | — |
| `ngxborg_repo_list` | List all repositories | — | — |
| `ngxborg_repo_delete` | Soft delete a repository (recoverable) | `tenant`, `name` | — |
| `ngxborg_repo_purge` | Permanently delete a repository (irreversible) | `tenant`, `name` | — |
| `ngxborg_repo_disable` | Disable a repository (blocks SSH keys) | `tenant`, `name` | — |
| `ngxborg_repo_enable` | Enable a disabled repository | `tenant`, `name` | — |

### System tools (3 tools)

| Tool | Description | Required arguments | Optional arguments |
|------|-------------|-------------------|-------------------|
| `ngxborg_version` | Show ngxborg version information | — | — |
| `ngxborg_doctor` | Run system diagnostics and health checks | — | — |
| `ngxborg_setup` | Initialize ngxborg on a fresh system | — | `adminPort`, `borgPort` |

## Example agent workflows

### Create a tenant and repository in one flow

An agent can chain tool calls to set up a complete backup environment:

```
1. ngxborg_user_create(username="alice", admin=false)
2. ngxborg_repo_create(tenant="alice", name="workstation")
3. ngxborg_user_key_add(username="alice", key="ssh-ed25519 AAAA...", label="alice@workstation")
```

### Audit all users and repositories

```
1. ngxborg_user_list()
2. ngxborg_repo_list()
```

### Disable a compromised user

```
1. ngxborg_user_disable(username="compromised_user")
```

This locks out all SSH keys and web UI access for that user instantly.

## Using with MCP clients

### Claude Desktop

Add ngxborg as a MCP server in your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "ngxborg": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-http"],
      "env": {
        "MCP_SERVER_URL": "http://172.16.1.107:8443/mcp"
      }
    }
  }
}
```

### Python (mcp SDK)

```python
from mcp import ClientSession, StdioServerParameters
from mcp.client.http import http_client

async with http_client("http://172.16.1.107:8443/mcp") as stdio:
    async with ClientSession(stdio[0], stdio[1]) as session:
        await session.initialize()
        tools = await session.list_tools()
        for tool in tools:
            print(f"  - {tool.name}: {tool.description}")
```

### GitHub Actions

Use the MCP endpoint from a workflow step to manage backup infrastructure:

```yaml
- name: Create backup repository
  run: |
    curl -s -X POST http://172.16.1.107:8443/mcp \
      -H "Content-Type: application/json" \
      -d '{
        "jsonrpc": "2.0",
        "id": 1,
        "method": "tools/call",
        "params": {
          "name": "ngxborg_repo_create",
          "arguments": {
            "tenant": "${{ github.actor }}",
            "name": "repo-${{ github.run_id }}"
          }
        }
      }'
```

## Security considerations

- **Anonymous access**: The `/mcp` endpoint does not require PAM authentication. Ensure it is only reachable from trusted networks or behind a reverse proxy with IP allowlisting, API keys, or mutual TLS.
- **No audit trail**: MCP tool calls are not logged separately from the web UI access logs. For compliance requirements, implement reverse proxy logging or a dedicated audit layer.
- **Privilege boundary**: MCP tools run with the same privileges as the web UI — as root. The `ngxborg-admin` group membership check applies to web UI admin endpoints but **not** to MCP tools. All MCP tools are effectively admin-level operations.
- **TLS mode**: If running with `--tls none` (plain HTTP), all MCP traffic is unencrypted. Use only on trusted internal networks or behind a TLS-terminating reverse proxy.

## Configuration

The MCP server is always enabled — there is no flag to disable it. It shares the same port and process as the web UI:

```
ngxborg web --addr :8443 --insecure
```

The systemd unit file is automatically configured by `ngxborg setup` to include the correct TLS flags:

```ini
ExecStart=/usr/local/bin/ngxborg web --addr :8443 --insecure
```

## Architecture

The MCP server is implemented in `internal/mcpserver/server.go` and uses the [mcp-go](https://github.com/mark3labs/mcp-go) library (v0.58.0) with Streamable HTTP transport. Each tool calls internal packages directly (`posix`, `borgrepo`, `sshaccess`, `provision`) rather than spawning CLI subprocesses, providing fast, structured responses with proper error handling.