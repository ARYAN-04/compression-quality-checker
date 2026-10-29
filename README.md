# can-i-compress-it

[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/Status-v0.4.1_Verified-00E676.svg)]()

A benchmarking harness that measures how MCP (Model Context Protocol) tool schema compression impacts function-calling accuracy in local and frontier language models.

It runs a **factorial evaluation matrix** (tasks &times; compression levels) against any OpenAI-compatible endpoint (LM Studio, Ollama, vLLM, llama.cpp, OpenRouter), validates tool calls with **BFCL-style AST matching**, computes **paired-bootstrap 95% confidence intervals**, identifies **failure cliffs**, and exports results as interactive terminal tables, JSON traces, Markdown matrices, and standalone HTML reports.

---

## Features

- **Full factorial matrix**: Evaluates models across 5 compression tiers (`none`, `low`, `medium`, `high`, `max`) over standard and custom task suites.
- **Interactive Bubble Tea TUI**: 5-step terminal wizard with live progress bars, streaming execution logs, and an interactive results explorer.
- **Dual compression backends**: Built-in deterministic Go compression engine or subprocess execution of the official Rust `mcp-compressor` binary.
- **11 bundled MCP schemas**: Baked-in static schemas for GitHub, Jira, Slack, SQLite, Filesystem, PostgreSQL, Memory, Fetch, Brave Search, Puppeteer, and Git for offline testing with zero external dependencies.
- **Robust inference and extraction**: Native `tool_calls` support with fallback extractors for Python function calls, code blocks, and XML tags.
- **Statistical rigor**: Paired bootstrap resampling across 1,000 iterations to isolate noise from true failure cliffs.
- **Multi-model regression**: `analyze` command merges multiple model traces to fit parameter scaling curves ($\text{Drop} \sim \alpha \log_{10}(B) + \beta$).

---

## Quick start

### 1. Build the binary

Requires Go 1.27 or newer:

```bash
# macOS / Linux:
go build -o can-i-compress-it ./cmd/can-i-compress-it

# Windows (PowerShell or CMD):
go build -o can-i-compress-it.exe .\cmd\can-i-compress-it
```

> **Windows Lab Assistants:** You can double-click **`scripts\run_windows.bat`** to run a fully automated smoke test and 150-cell matrix sweep against LM Studio with zero command-line setup. See the [Windows Lab Guide](docs.md#9-windows-lab-assistant-guide-turnkey--lm-studio) for step-by-step instructions.

### 2. Run a smoke test

Verify your model connection with an isolated single-tool check:

```bash
# Test against a local OpenAI-compatible endpoint (LM Studio / Ollama)
./can-i-compress-it test \
  --endpoint http://localhost:1234/v1 \
  --model qwen2.5-7b-instruct \
  --prompt "Get issue 42 in repo torvalds/linux" \
  --level none

# Or test with the deterministic offline mock (no server required)
./can-i-compress-it test --mock -m mock-model -p "Get issue 42"
```

### 3. Launch the interactive wizard

```bash
./can-i-compress-it tui
```

Press `[F]` on Step 1 to immediately launch the 150-cell benchmark matrix on `synthetic_v1`.

### 4. Run headlessly via CLI

```bash
./can-i-compress-it run \
  --model qwen2.5-7b-instruct \
  --params-b 7.0 \
  --endpoint http://localhost:1234/v1 \
  --levels none,low,medium,high,max \
  --output ./results/results.json \
  --html-report ./results/report.html \
  --md-table ./results/table.md
```

---

## Compression tiers

| Level | Schema transformation | Token savings | Fragility profile |
|---|---|---|---|
| `none` | Raw uncompressed JSON Schema | 0% (baseline) | Reference baseline |
| `low` | Prunes description whitespace, drops optional property titles | 15% to 30% | Low |
| `medium` | Summarizes descriptions, prunes optional property schemas | 35% to 55% | Moderate (types binding errors) |
| `high` | Retains only tool names and compact parameter signatures | 60% to 80% | High (semantic confusion) |
| `max` | Replaces all tools with `get_tool_schema` and `invoke_tool` meta-tools | 70% to 97% | Severe (requires 2-step planning) |

---

## Documentation

For comprehensive guides, setup details, and advanced workflows, see **[docs.md](docs.md)**:

- [Setting up model backends (LM Studio, Ollama, vLLM, Mock)](docs.md#2-setting-up-model-backends)
- [Wrapping the real `mcp-compressor` binary](docs.md#5-using-the-real-mcp-compressor-binary)
- [Working with bundled offline MCP server suites](docs.md#6-working-with-bundled-and-custom-mcp-suites)
- [Cross-model comparison and scaling regressions](docs.md#7-multi-model-analysis-and-scaling-regressions)
- [Output file management and multi-trace workflows](docs.md#8-output-file-management-and-multi-trace-workflows)
- [Windows Lab Assistant Guide (Turnkey & LM Studio)](docs.md#9-windows-lab-assistant-guide-turnkey--lm-studio)
- [Complete CLI command and flag reference](docs.md#command-reference)

---

## Architecture

```
cmd/can-i-compress-it/
  main.go                 Application entrypoint
internal/
  cmd/                    Cobra CLI wiring (tui, run, test, suite, serve-mcp, analyze, report)
  tui/                    Bubble Tea wizard, live background runner, results explorer
  domain/                 Shared domain entities, compression levels, matrix interfaces
  suite/                  Synthetic task generator, distractor padding, 11 embedded schemas
  mcp/                    Subprocess proxy, builtin compressor, meta-tools, serve-mcp bridge
  inference/              OpenAI-compatible client with tool extraction & deterministic mock
  eval/                   BFCL structural AST matcher & multi-hop chain verifier
  runner/                 Factorial matrix scheduler & active-level prompt injector
  stats/                  Paired bootstrap intervals, failure cliff detection, log regression
  report/                 Terminal tables, Markdown tables, standalone HTML reports
```

---

## Development

```bash
# Run unit and integration tests
go test -v ./...

# Run static analysis
go vet ./...

# Format codebase
gofmt -s -w .
```

---

## License

Distributed under the [MIT License](LICENSE).
