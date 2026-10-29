# Benchmark and operational guide

This guide covers setup, execution modes, MCP schema handling, multi-model evaluation, and command references for `can-i-compress-it`.

## Table of contents

- [How it works](#how-it-works)
- [Step-by-step walkthrough](#step-by-step-walkthrough)
  - [1. Building the harness](#1-building-the-harness)
  - [2. Setting up model backends](#2-setting-up-model-backends)
  - [3. Running a smoke test](#3-running-a-smoke-test)
  - [4. Running the full benchmark matrix](#4-running-the-full-benchmark-matrix)
  - [5. Using the real mcp-compressor binary](#5-using-the-real-mcp-compressor-binary)
  - [6. Working with bundled and custom MCP suites](#6-working-with-bundled-and-custom-mcp-suites)
  - [7. Multi-model analysis and scaling regressions](#7-multi-model-analysis-and-scaling-regressions)
  - [8. Output file management and multi-trace workflows](#8-output-file-management-and-multi-trace-workflows)
  - [9. Windows Lab Assistant Guide (Turnkey & LM Studio)](#9-windows-lab-assistant-guide-turnkey--lm-studio)
- [Bundled popular MCP tool suites](#bundled-popular-mcp-tool-suites)
- [Compression levels](#compression-levels)
- [Evaluation metrics and scoring](#evaluation-metrics-and-scoring)
- [Command reference](#command-reference)

---

## How it works

`can-i-compress-it` evaluates how information loss in compressed tool schemas affects function-calling accuracy.

```
[ Model Endpoint ] <── (HTTP /v1/chat/completions) ── [ Scheduler Loop ]
                                                             │
  ┌──────────────────────────────────────────────────────────┴──────────────────────────────┐
  ▼                                                                                         ▼
[ Built-in Go Engine ]                                                    [ Subprocess Proxy ]
  Transforms schema in-memory                                               Spawns mcp-compressor CLI
  (none, low, medium, high, max)                                            behind stdio serve-mcp bridge
```

### Execution pipeline

1. **Schema transformation**: For each level in the matrix, the active toolset is compressed via the built-in pure Go engine or by spawning `mcp-compressor -c <level> -- <target>`.
2. **Context injection**: The active compressed schema is injected as JSON into the system prompt context. This ensures models without custom function-calling templates receive their available tools. For level `max`, available tool catalogue names are listed so the model knows which names to query with `get_tool_schema`.
3. **Turn execution**: The model receives task instructions. The client supports native `tool_calls` payloads and fallback text extractors for code blocks, Python function syntax, and XML tags.
4. **AST validation**: Tool selection and parameter values are evaluated with BFCL-style structural comparison. String representations of numbers and booleans are coerced in standard mode.
5. **Statistical synthesis**: Paired bootstrap resampling computes 95% confidence intervals on accuracy drops against the uncompressed baseline to identify failure cliffs.

---

## Step-by-step walkthrough

### 1. Building the harness

Requirements: Go 1.27 or newer.

```bash
# Clone the repository
git clone git@github.com:ARYAN-04/compression-quality-checker.git
cd compression-quality-checker

# Build standalone binary (macOS / Linux)
go build -o can-i-compress-it ./cmd/can-i-compress-it
./can-i-compress-it --help

# Build standalone binary (Windows PowerShell or CMD)
go build -o can-i-compress-it.exe .\cmd\can-i-compress-it
.\can-i-compress-it.exe --help

# Cross-compile Windows binary from macOS/Linux for lab distribution
GOOS=windows GOARCH=amd64 go build -o can-i-compress-it.exe ./cmd/can-i-compress-it
```

---

### 2. Setting up model backends

The harness connects to any OpenAI-compatible `/v1/chat/completions` endpoint.

#### LM Studio (local or network IP)
1. Load a model supporting tool calling (such as `Qwen2.5-7B-Instruct` or `Llama-3.1-8B-Instruct`).
2. Start the local server on port `1234`.
3. Base URLs normalize automatically: `http://localhost:1234`, `http://localhost:1234/v1`, or `http://192.168.1.50:1234` all resolve to `/v1/chat/completions`.

#### Ollama
```bash
ollama run qwen2.5:7b
# Endpoint: http://localhost:11434/v1
```

#### Deterministic offline mock
Pass `--mock` in CLI commands or select the **Offline Mock** preset in the TUI to run benchmarks without a running model server.

---

### 3. Running a smoke test

Run an isolated tool call to verify connection, response format, real latency, and schema handling:

```bash
# Test against LM Studio with embedded GitHub schema (default)
./can-i-compress-it test \
  --endpoint http://localhost:1234/v1 \
  --model qwen2.5-7b-instruct \
  --prompt "Get issue 42 in repo torvalds/linux" \
  --schema github \
  --level none

# Test against Ollama with medium compression on PostgreSQL tools
./can-i-compress-it test \
  --endpoint http://localhost:11434/v1 \
  --model llama3.1:8b \
  --prompt "List all database tables in public schema" \
  --schema postgres \
  --level medium

# Test two-stage meta-tool planning at max compression (get_tool_schema -> invoke_tool)
./can-i-compress-it test \
  --endpoint http://localhost:1234/v1 \
  --model qwen2.5-7b-instruct \
  --prompt "Get issue 42 in repo torvalds/linux" \
  --level max

# Test against offline mock
./can-i-compress-it test --mock -m mock-model -p "Get issue 42"
```

The `--schema` flag accepts bundled schemas (`github`, `jira`, `slack`, `sqlite`, `filesystem`, `postgres`, `memory`, `fetch`, `brave-search`, `puppeteer`, `git`, or `all`) as well as file paths to custom schema JSON files. You can also specify `--suite <path.json>` to load tools from a task suite.

Expected output:
```text
tool:    get_issue
args:    map[issue_number:42 owner:torvalds repo:linux]
latency: 340ms
result:  map[ok:true result_id:res-1-811c9dc5 tool:get_issue]
```

---

### 4. Running the full benchmark matrix

The canonical matrix evaluates 30 tasks from `synthetic_v1` across all 5 compression levels for 150 cell evaluations.

#### Option A. Interactive Bubble Tea TUI wizard
Launch the terminal UI:
```bash
./can-i-compress-it tui
```

Wizard navigation:
- **Step 1 (Provider & Model)**: Configure model name and endpoint. Press `[F]` to launch the full 150-cell matrix immediately.
- **Step 2 (MCP Suite)**: Choose between `synthetic_v1`, 11 pre-annotated server templates, synthetic generation, live MCP servers, or custom JSON suites.
- **Step 3 (Compression & Targets)**: Select levels, choose compressor backend, and set output paths for JSON trace, HTML report, and Markdown table.
- **Step 4 (Live Runner)**: Monitor progress bar, token counters, live latency, and recent streaming cells.
- **Step 5 (Results Explorer)**: View color-coded safety table, detected failure cliffs, and confidence intervals. Press `[O]` to open the HTML report in a browser, `[M]` to export Markdown, or `[T]` to inspect per-task logs.

#### Option B. CLI matrix sweep
Run headlessly:
```bash
./can-i-compress-it run \
  --model qwen2.5-7b-instruct \
  --params-b 7.0 \
  --endpoint http://localhost:1234/v1 \
  --levels none,low,medium,high,max \
  --output ./results/qwen_results.json \
  --html-report ./results/qwen_report.html \
  --md-table ./results/safety_matrix.md
```

---

### 5. Using the real mcp-compressor binary

By default, the harness uses an internal Go compression engine. To benchmark the official Rust `mcp-compressor` binary:

1. Install `mcp-compressor` (for example, via `cargo install mcp-compressor`).
2. Pass `--compressor` pointing to the binary path (tilde expansion like `~/.cargo/bin/mcp-compressor` is supported):

```bash
./can-i-compress-it run \
  --model qwen2.5-7b-instruct \
  --params-b 7.0 \
  --endpoint http://localhost:1234/v1 \
  --compressor ~/.cargo/bin/mcp-compressor \
  --output ./results/real_compressor_results.json
```

When evaluating offline suites with `mcp-compressor`, the harness automatically bridges the suite over standard input/output using an internal `serve-mcp` JSON-RPC server behind `mcp-compressor -- <binary> serve-mcp`.

---

### 6. Working with bundled and custom MCP suites

#### Using bundled offline schemas
11 popular MCP server schemas are baked into the binary with `//go:embed`. Benchmark them without installing Node or configuring servers:

```bash
# Run against embedded GitHub schema
./can-i-compress-it run \
  --model qwen2.5-7b-instruct \
  --suite github \
  --output ./results/github_run.json

# Run against embedded PostgreSQL schema
./can-i-compress-it run \
  --model qwen2.5-7b-instruct \
  --suite postgres \
  --output ./results/postgres_run.json
```

#### Importing static schema JSON files
Generate benchmark task suites from any standard MCP schema file:

```bash
./can-i-compress-it suite import \
  --from-schema ./suites/schemas/slack.json \
  -o ./suites/slack_suite.json
```

#### Synchronizing schemas with Python
Export or refresh bundled MCP schemas using `uv`:

```bash
# Export all 11 bundled schemas to disk
uv run scripts/fetch_mcp_schemas.py --all

# Interrogate a live MCP server over stdio
uv run scripts/fetch_mcp_schemas.py --cmd "npx -y @modelcontextprotocol/server-git" -o ./suites/schemas/git.json
```

#### Ingesting live MCP servers
```bash
./can-i-compress-it suite import \
  --template sqlite \
  -o ./suites/sqlite_suite.json \
  -- npx -y @modelcontextprotocol/server-sqlite /tmp/test.db
```

---

### 7. Multi-model analysis and scaling regressions

Compare multiple models, evaluate cross-model failure cliffs, and fit parameter scaling equations:

```bash
./can-i-compress-it analyze \
  -i "results/*.json" \
  --bootstrap-iterations 1000 \
  --output-table ./results/cross_model_summary.md \
  --output-json ./results/cross_model_data.json
```

---

### 8. Output file management and multi-trace workflows

By default, single runs write to:
- `./results/results.json`
- `./results/report.html`
- `./results/table.md`

When benchmarking several models for comparison in `analyze`, specify unique filenames for each model run:

```bash
# Model 1
./can-i-compress-it run --model smollm-135m --params-b 0.1 --output ./results/smollm_0.1b.json

# Model 2
./can-i-compress-it run --model qwen2.5-7b --params-b 7.0 --output ./results/qwen_7b.json

# Compare all traces
./can-i-compress-it analyze -i "results/*.json" --output-table ./results/comparison.md
```

Re-render reports from existing trace files without re-running models:

```bash
./can-i-compress-it report \
  -i ./results/results.json \
  --html-report ./results/report.html \
  --md-table ./results/table.md
```

#### Diagnostic logging during TUI runs
During interactive TUI benchmarks, standard logs are discarded so the Bubble Tea terminal display is not corrupted. Set the `CAN_I_COMPRESS_IT_LOG` environment variable to record full runtime execution details and model warnings:

```bash
# macOS / Linux
export CAN_I_COMPRESS_IT_LOG="./runner.log"
./can-i-compress-it tui

# Windows PowerShell
$env:CAN_I_COMPRESS_IT_LOG = ".\runner.log"
.\can-i-compress-it.exe tui
```

---

### 9. Windows Lab Assistant Guide (Turnkey & LM Studio)

This walkthrough is designed for lab assistants running benchmarks on Windows test machines without needing deep knowledge of the harness internals.

#### Step 1: Start LM Studio
1. Launch **LM Studio** on the Windows machine.
2. Under the **Developer** tab (left sidebar), click **Load Model** and select the designated evaluation model (e.g. `Qwen2.5-7B-Instruct`).
3. Click the green **Start Server** button. Ensure the port is `1234` (the default `http://localhost:1234/v1`).
4. Keep LM Studio running in the background.

#### Step 2: Run the automated turnkey script
A pre-configured Windows script is provided in `scripts\run_windows.bat`.

1. Double-click **`scripts\run_windows.bat`** (or execute in Command Prompt / PowerShell: `.\scripts\run_windows.bat`).
2. The script automatically:
   - Verifies or compiles `can-i-compress-it.exe`.
   - Runs a smoke test to confirm LM Studio is responding.
   - Executes the 150-cell benchmark sweep across all 5 compression levels (`none`, `low`, `medium`, `high`, `max`).
   - Automatically opens the resulting interactive HTML report in your web browser (Edge/Chrome).
3. To test a different model name or remote machine IP, pass them as arguments:
   ```cmd
   .\scripts\run_windows.bat llama-3.1-8b-instruct http://127.0.0.1:1234/v1
   ```

#### Step 3: Run via the interactive TUI wizard
If preferred, launch the full-screen terminal wizard:
```powershell
.\can-i-compress-it.exe tui
```
- On **Step 1**, verify your model name and press **`[F]`** to immediately trigger the full 150-cell benchmark.
- Once finished, press **`[O]`** to open the HTML report in your browser.

#### Step 4: Run headlessly via PowerShell
For automated or scripted sweeps:
```powershell
.\can-i-compress-it.exe run `
  --endpoint http://localhost:1234/v1 `
  --model qwen2.5-7b-instruct `
  --levels none,low,medium,high,max `
  --output .\results\qwen_results.json `
  --html-report .\results\qwen_report.html `
  --md-table .\results\qwen_table.md
```

All results are saved in the `results\` folder. Lab assistants can simply copy the generated `.html` and `.json` files to submit evaluation results.

---

## Bundled popular MCP tool suites

The binary includes 11 static schema templates:

| Server | Tool count | Key tools |
|---|---|---|
| **GitHub** | 7 | `get_issue`, `list_issues`, `create_issue`, `get_pull_request`, `list_commits`, `create_pull_request_comment`, `search_repositories` |
| **Jira** | 5 | `get_issue`, `create_issue`, `update_issue_status`, `add_comment`, `search_issues_jql` |
| **Slack** | 5 | `list_channels`, `post_message`, `reply_to_thread`, `get_channel_history`, `list_users` |
| **SQLite** | 4 | `list_tables`, `describe_table`, `read_query`, `execute_query` |
| **Filesystem** | 7 | `list_directory`, `read_file`, `search_files`, `write_file`, `move_file`, `get_file_info`, `delete_file` |
| **PostgreSQL** | 5 | `list_schemas`, `list_tables`, `describe_table`, `read_query`, `execute_query` |
| **Memory** | 5 | `create_entities`, `open_nodes`, `create_relations`, `search_nodes`, `read_graph` |
| **Fetch** | 3 | `fetch`, `extract_markdown`, `extract_headers` |
| **Brave Search** | 2 | `brave_web_search`, `brave_local_search` |
| **Puppeteer** | 5 | `navigate`, `screenshot`, `click`, `fill`, `evaluate` |
| **Git** | 5 | `git_status`, `git_diff`, `git_commit`, `git_log`, `git_branch` |

---

## Compression levels

| Level | Transformation description | Approximate token reduction |
|---|---|---|
| `none` | Raw uncompressed JSON Schema definitions | 0% (baseline) |
| `low` | Removes description whitespace and drops non-essential property titles | 15% to 30% |
| `medium` | Summarizes descriptions and prunes optional property definitions | 35% to 55% |
| `high` | Retains only tool names and compact parameter type signatures | 60% to 80% |
| `max` | Replaces all tools with `get_tool_schema` and `invoke_tool` meta-tools | 70% to 97% |

---

## Evaluation metrics and scoring

- **Tool selection**: Exact match between the invoked tool name and ground truth.
- **Parameter fill**: AST-normalized structural comparison validating required keys, types, and values while ignoring key ordering and whitespace.
- **Multi-hop completion**: Multi-turn validation with dynamic variable threading across dependent tool steps.
- **Failure cliff**: Lowest compression level where accuracy drops 10 percentage points or more below baseline.
- **95% Bootstrap CI**: Paired bootstrap resampling across 1,000 iterations to quantify variance.

---

## Command reference

| Command | Purpose | Key flags |
|---|---|---|
| `can-i-compress-it tui` | Full-screen interactive wizard | (none) |
| `can-i-compress-it run` | Headless matrix benchmark | `--model`, `--endpoint`, `--levels`, `--params-b`, `--compressor`, `--suite`, `--output`, `--html-report`, `--md-table`, `--mock` |
| `can-i-compress-it test` | Single tool verification probe | `-m, --model`, `-p, --prompt`, `--endpoint`, `--level`, `--schema`, `--suite`, `--compressor`, `--mock` |
| `can-i-compress-it suite generate` | Create synthetic benchmark suites | `-n, --num-tasks`, `--ambiguity`, `--prompt-style`, `--distractors`, `--from-schema`, `--seed`, `-o, --output` |
| `can-i-compress-it suite import` | Ingest tools from schema files or live MCP processes | `--template`, `--from-schema`, `-o, --output`, `-- <cmd>` |
| `can-i-compress-it analyze` | Multi-model bootstrap and regression analysis | `-i, --inputs`, `--bootstrap-iterations`, `--output-table`, `--output-json` |
| `can-i-compress-it report` | Render HTML and Markdown from existing JSON traces | `-i, --input`, `--html-report`, `--md-table` |
