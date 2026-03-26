# checkredirects-mcp

MCP (Model Context Protocol) server for [checkredirects.io](https://checkredirects.io). Lets AI assistants like Claude inspect redirect chains, batch check URLs, compare user agents, manage monitors, and export to Google Sheets.

## Install

```bash
go install github.com/neondeerdatalabs/checkredirects-mcp@latest
```

## Setup

### Claude Desktop

Add to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "checkredirects": {
      "command": "checkredirects-mcp",
      "env": {
        "CHECKREDIRECTS_API_KEY": "httpd_your_key_here"
      }
    }
  }
}
```

### Claude Code

```bash
claude mcp add checkredirects -- checkredirects-mcp
```

Then set your API key:
```bash
export CHECKREDIRECTS_API_KEY=httpd_your_key_here
```

## Available tools

| Tool | Description |
|------|-------------|
| `check_url` | Quick check -- where does this URL end up? Returns final URL, status, hops, and a chain summary. Best for 1-3 URLs. |
| `inspect_url` | Full inspection -- every hop with headers, timing, TLS certs, geo. Use when you need the technical details. |
| `compare_agents` | Check a URL with different user agents (Googlebot, Chrome, social crawlers). Spot cloaking or mobile redirect issues. |
| `batch_check_and_wait` | Submit up to 500 URLs, wait for completion, return simplified results. The main tool for bulk checking. |
| `batch_results` | Re-fetch results from a previous batch by job ID. |
| `list_monitors` | List recurring URL check monitors. |
| `create_monitor` | Schedule URLs to be checked on an interval (min 30 minutes). |
| `export_to_sheets` | Export batch results to Google Sheets. |

## Example prompts

**Quick check:**
> "Where does flatfile.io redirect to?"

**Batch check:**
> "Check all these URLs and tell me which ones have redirect chains: [url1, url2, ...]"

**User agent comparison:**
> "Does this URL redirect differently for Googlebot vs Chrome?"

**Migration QA:**
> "Check these 200 old URLs and tell me which ones are broken or redirecting to the wrong place."

**Monitoring:**
> "Set up a weekly monitor on our top 50 pages and export results to Google Sheets."

## Configuration

| Environment variable | Description |
|---------------------|-------------|
| `CHECKREDIRECTS_API_KEY` | Your API key (required). Get one at app.checkredirects.io/settings/api-keys |
| `CHECKREDIRECTS_API_URL` | API base URL (default: `https://api.checkredirects.io`) |

## Get an API key

1. Sign up at [app.checkredirects.io](https://app.checkredirects.io/signup)
2. Go to Settings > API Keys
3. Create a new key (starts with `httpd_`)

Free plan includes 100 checks/month.

## License

MIT
