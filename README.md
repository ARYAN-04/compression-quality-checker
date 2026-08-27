# can-i-compress-it

[![Go Version](https://img.shields.io/badge/Go-1.27+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE)
[![Status](https://img.shields.io/badge/Status-v0.4.0_Verified-00E676.svg)]()

An end-to-end benchmarking harness that measures how MCP (Model Context Protocol) tool-schema compression impacts tool-use accuracy in language models.

It executes a **full factorial evaluation matrix** (evaluation tasks &times; compression levels) against any OpenAI-compatible endpoint (LM Studio, Ollama, vLLM, llama.cpp, OpenRouter), scores every tool call with **BFCL-style AST-normalized validation**, computes **paired-bootstrap 95% confidence intervals**, detects per-model **failure cliffs**, and emits schema-compression safety guidance as interactive terminal tables, JSON traces, Markdown matrices, and responsive standalone HTML reports.

---

## Table of Contents

- [Why This Exists](#why-this-exists)
- [End-to-End Walkthrough](#end-to-end-walkthrough)
  - [Step 1: Build the Harness](#step-1-build-the-harness)
  - [Step 2: Start Your Model Backend (LM Studio / Ollama / Mock)](#step-2-start-your-model-backend)
  - [Step 3: Run a 10-Second Smoke Test](#step-3-run-a-10-second-smoke-test)
  - [Step 4: Run the Full Factorial Benchmark Matrix](#step-4-run-the-full-factorial-benchmark-matrix)
    - [Option A: Interactive Bubble Tea TUI Wizard](#option-a-interactive-bubble-tea-tui-wizard)
    - [Option B: CLI Full Matrix Sweep](#option-b-cli-full-matrix-sweep)
  - [Step 5: Run with the Real `mcp-compressor` Binary](#step-5-run-with-the-real-mcp-compressor-binary)
  - [Step 6: Benchmark Live MCP Servers & Templates](#step-6-benchmark-live-mcp-servers--templates)
  - [Step 7: Statistical Analysis & Cross-Model Regressions](#step-7-statistical-analysis--cross-model-regressions)
- [Compression Levels](#compression-levels)
- [Evaluation Metrics & Scoring](#evaluation-metrics--scoring)
- [Command Reference](#command-reference)
- [Architecture](#architecture)
- [Development & Verification](#development--verification)

---

## Why This Exists

MCP tool catalogues routinely consume **10k–20k tokens of JSON schema** before an LLM agent reasons a single step. Tool compressors (like Atlassian's `mcp-compressor`) reduce schema size by **70% to 97%** — but their published accuracy benchmarks are validated on frontier-scale models (Claude 3.5 Sonnet, GPT-4o).

Small and local models (**0.5B to 14B parameters**) are significantly more fragile when descriptions, optional parameters, or full schema structures are stripped. This harness empirically discovers the exact compression tier where a model begins hallucinating or dropping parameters.

---

## End-to-End Walkthrough

### Step 1: Build the Harness

**Requirements**: Go 1.27 or newer.

```bash
# Clone the repository and navigate into it
cd compression-quality-checker

# Build the standalone binary
go build -o can-i-compress-it ./cmd/can-i-compress-it

# Verify build
./can-i-compress-it --help
```

---

### Step 2: Start Your Model Backend

`can-i-compress-it` connects to any standard OpenAI-compatible `/v1/chat/completions` server.

#### A. LM Studio (Local or Remote Windows PC)
1. In LM Studio, load a function-calling model (e.g. `Qwen/Qwen2.5-7B-Instruct-GGUF` or `Meta-Llama-3.1-8B-Instruct-GGUF`).
2. Navigate to the **Developer / Local Server** tab (port `1234`).
3. If running on a separate Windows machine on your local network:
   - Toggle **Serve on Local Network (0.0.0.0)**.
   - Ensure Windows Firewall allows inbound TCP traffic on port `1234`.
4. The harness features **automatic URL normalization**: supplying `http://192.168.1.50:1234`, `http://192.168.1.50:1234/v1`, or `http://192.168.1.50:1234/v1/chat/completions` resolves automatically.

#### B. Ollama
```bash
ollama run qwen2.5:7b
# Endpoint: http://localhost:11434/v1
```

#### C. Deterministic Offline Mock (No GPU Required)
Pass `--mock` (or select the **Offline Mock** preset in the TUI) to simulate deterministic responses without running a local server.

---

### Step 3: Run a 10-Second Smoke Test

Probe your endpoint with an isolated tool call to verify connection, schema parsing, and latency:

```bash
# Against LM Studio (local or remote Windows IP):
./can-i-compress-it test \
  --endpoint http://localhost:1234/v1 \
  --model qwen2.5-7b-instruct \
  --prompt "Get issue 42 in repo torvalds/linux" \
  --level none

# Against Ollama:
./can-i-compress-it test \
  --endpoint http://localhost:11434/v1 \
  --model llama3.1:8b \
  --prompt "Search for issues matching payment retry" \
  --level medium

# Against offline mock:
./can-i-compress-it test --mock -m mock-model -p "Get issue 42"
```

**Expected output**:
```text
tool:    get_issue
args:    map[issue_number:42 owner:torvalds repo:linux]
latency: 340ms
result:  map[ok:true result_id:res-1-811c9dc5 tool:get_issue]
```

---

### Step 4: Run the Full Factorial Benchmark Matrix

The canonical benchmark matrix tests the model across **all 5 compression levels** (`none`, `low`, `medium`, `high`, `max`) over the standard **`synthetic_v1` suite** (30 tasks covering Ambiguous Names, Overlapping Schemas, and Multi-Hop Chains) for a total of **150 factorial evaluation cells**.

#### Option A: Interactive Bubble Tea TUI Wizard

Launch the full-screen terminal wizard:

```bash
./can-i-compress-it tui
```

```
⚡ CAN-I-COMPRESS-IT — MCP Schema Compression Benchmark Wizard

[1. Provider & Model] ──  2. MCP Suite  ──  3. Compression  ──  4. Live Runner  ──  5. Results
```

1. **Step 1 (Provider & Model)**:
   - LM Studio is selected by default (`qwen2.5-7b-instruct`, `http://localhost:1234/v1`, 7.0B).
   - Press **`[F]`** at any time to **instantly launch the 150-cell Full Factorial Matrix**!
   - Or press **`[Enter]`** to customize wizard steps.
2. **Step 2 (MCP Suite)**:
   - Tab 1 `[1] Full Factorial Matrix (synthetic_v1)` is selected by default.
   - You can also select pre-annotated MCP server templates (`[2]`), synthetic generator (`[3]`), live MCP server (`[4]`), or custom JSON (`[5]`).
3. **Step 3 (Compression & Targets)**:
   - All 5 compression tiers are checked by default.
   - Choose **Built-in Pure Go** or **Subprocess Binary (`mcp-compressor`)**.
   - Output paths default to `./results/results.json`, `./results/report.html`, `./results/table.md`.
4. **Step 4 (Live Execution Monitor)**:
   - Streams live progress with Bubbles progress bar, status spinner, live accuracy percentages, average latency, and recent cell results table.
5. **Step 5 (Results Explorer & Safety Table)**:
   - Displays the Lipgloss safety matrix table with color-coded accuracy cells.
   - Highlights observed **Failure Cliffs** (e.g. `>= high`) and 95% bootstrap confidence intervals.
   - Press **`[O]`** to launch the standalone HTML report in your browser.
   - Press **`[M]`** to export the Markdown safety table.
   - Press **`[T]`** to inspect per-task execution details.
   - Press **`[R]`** to rerun or adjust parameters.

---

#### Option B: CLI Full Matrix Sweep

Run the full factorial matrix headlessly via CLI:

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

**Terminal Output**:
```
wrote 150 cell results to ./results/qwen_results.json

Model                 Params (B)  none      low       medium    high      max       Failure Cliff
──────────────────────────────────────────────────────────────────────────────────────────────────
qwen2.5-7b-instruct   7.0B        100.0%    100.0%     93.3%    83.3%     56.7%     >= high

html report:    ./results/qwen_report.html
markdown table: ./results/safety_matrix.md
```

---

### Step 5: Run with the Real `mcp-compressor` Binary

By default, the harness uses a pure Go deterministic compression engine matching Atlassian's `mcp-compressor` specification. To wrap and benchmark the real `mcp-compressor` binary:

1. **Obtain or build `mcp-compressor`**:
   ```bash
   # Ensure mcp-compressor is installed in your PATH or target directory
   which mcp-compressor
   ```

2. **Pass `--compressor` to `run` or `test`**:
   ```bash
   ./can-i-compress-it run \
     --model qwen2.5-7b-instruct \
     --params-b 7.0 \
     --endpoint http://localhost:1234/v1 \
     --compressor /usr/local/bin/mcp-compressor \
     --output ./results/real_compressor_results.json \
     --html-report ./results/real_compressor_report.html
   ```

3. **In the TUI**:
   In **Step 3 (Compression)**, toggle **Compressor Backend** to **Subprocess Binary (`mcp-compressor`)** and provide the binary path.

---

### Step 6: Benchmark Live MCP Servers & Templates

#### Ingest a Live MCP Server via Stdio
Ingest tool schemas directly from a live MCP server process and evaluate compression behavior against realistic production schemas:

```bash
# Ingest SQLite MCP server
./can-i-compress-it suite import \
  --template sqlite \
  -o ./suites/sqlite_suite.json \
  -- npx -y @modelcontextprotocol/server-sqlite /tmp/test.db

# Ingest Filesystem MCP server
./can-i-compress-it suite import \
  --template filesystem \
  -o ./suites/fs_suite.json \
  -- npx -y @modelcontextprotocol/server-filesystem /tmp

# Run the matrix against the imported suite
./can-i-compress-it run \
  --model qwen2.5-7b-instruct \
  --suite ./suites/sqlite_suite.json \
  --output ./results/sqlite_benchmark.json
```

#### Synthesize Custom Suites with Distractor Padding
```bash
# Generate 50 intent-style tasks padded with 30 distractor tools
./can-i-compress-it suite generate \
  -n 50 \
  --ambiguity high \
  --prompt-style intent \
  --distractors 30 \
  --seed 42 \
  -o ./suites/custom_stress.json
```

---

### Step 7: Statistical Analysis & Cross-Model Regressions

Compare multiple model runs, compute paired-bootstrap confidence intervals, and fit log-scale parameter scaling curves ($\text{Accuracy Drop} \sim \alpha \log_{10}(B) + \beta$):

```bash
# Analyze all completed runs in results/
./can-i-compress-it analyze \
  -i "results/*.json" \
  --bootstrap-iterations 1000 \
  --output-table ./results/combined_safety.md \
  --output-json ./results/cross_model_analysis.json
```

**Analysis Output**:
```
Cross-Model Failure Cliffs & Regression:

Model                 Params (B)  Baseline  Cliff Level  Cliff Acc  Drop (pp)   95% Bootstrap CI
────────────────────────────────────────────────────────────────────────────────────────────────
smollm-135m           0.1B        66.7%     >= low       50.0%      -16.7%      [-26.7%,  -6.7%]
qwen2.5-0.5b          0.5B        83.3%     >= medium    63.3%      -20.0%      [-30.0%, -10.0%]
qwen2.5-7b-instruct   7.0B        100.0%    >= high      83.3%      -16.7%      [-26.7%,  -6.7%]
llama-3.1-70b         70.0B       100.0%    NONE         96.7%       -3.3%      [ -6.7%,   0.0%]

Parameter Scale Fit: Drop(High) = -34.2 + 8.4 * log10(ParamsB)  (R² = 0.94)
```

---

## Compression Levels

| Level | Schema Transformation | Token Savings | Small-Model Fragility Factor |
|---|---|---|---|
| `none` | Raw uncompressed JSON Schema | 0% (baseline) | None |
| `low` | Trims redundant whitespace, drops optional property titles | 15–30% | Low |
| `medium` | Summarizes descriptions, keeps required property definitions | 35–55% | Moderate (types binding errors) |
| `high` | Retains only tool names and parameter type signatures | 60–80% | High (semantic confusion) |
| `max` | Replaces all tools with `get_tool_schema` and `invoke_tool` meta-tools | 70–97% | Severe (requires 2-step planning) |

---

## Evaluation Metrics & Scoring

- **Tool Selection Accuracy**: Exact match between the model's invoked tool and expected ground truth.
- **Parameter Fill Accuracy**: AST-normalized structural comparison of argument payloads (validating types, required keys, enum values; order- and whitespace-insensitive).
- **Multi-Hop Chain Completion**: Verifies sequential multi-turn execution with dynamic variable threading (e.g. `<from:step1.result_id>` passed to step 2).
- **Failure Cliff**: The lowest compression intensity where accuracy drops **&ge;10 percentage points** below the model's uncompressed baseline.
- **95% Bootstrap Confidence Interval**: 1,000 paired resampling iterations quantifying variance and noise in observed accuracy drops.

---

## Command Reference

| Command | Description | Key Flags |
|---|---|---|
| `can-i-compress-it tui` | Interactive 5-step guided wizard & live safety explorer | (none) |
| `can-i-compress-it run` | Full factorial matrix execution | `--model`, `--endpoint`, `--levels`, `--params-b`, `--compressor`, `--suite`, `--output`, `--html-report`, `--md-table`, `--mock` |
| `can-i-compress-it test` | Isolated single-tool verification probe | `-m, --model`, `-p, --prompt`, `--endpoint`, `--level`, `--compressor`, `--mock` |
| `can-i-compress-it suite generate` | Synthesize deterministic benchmark suites | `-n, --num-tasks`, `--ambiguity`, `--prompt-style`, `--distractors`, `--seed`, `-o, --output` |
| `can-i-compress-it suite import` | Ingest live MCP server tools via stdio | `--template`, `-o, --output`, `-- <mcp-server-command>` |
| `can-i-compress-it analyze` | Cross-model bootstrap & regression analysis | `-i, --inputs`, `--bootstrap-iterations`, `--output-table`, `--output-json` |
| `can-i-compress-it report` | Re-render reports without recomputation | `-i, --input`, `--html`, `--md` |

---

## Architecture

```
cmd/can-i-compress-it/
  main.go                 Application entrypoint
internal/
  cmd/                    Cobra CLI wiring (tui, run, test, suite, analyze, report)
  tui/                    Bubble Tea wizard, live background runner, results explorer
  domain/                 Shared domain entities, compression levels, matrix interfaces
  suite/                  Synthetic task generator, distractor padding, MCP templates
  mcp/                    JSON-RPC proxy, builtin compressor, meta-tools, stdio client
  inference/              OpenAI-compatible HTTP client (LM Studio/Ollama) & deterministic mock
  eval/                   BFCL structural AST matcher & multi-hop chain verifier
  runner/                 Factorial matrix scheduler & agent turn loop
  stats/                  Paired bootstrap intervals, failure cliff detection, log-scale regression
  report/                 Lipgloss terminal tables, Markdown tables, standalone HTML reports
```

---

## Development & Verification

```bash
# Run all unit and integration test suites
go test -v ./...

# Run static analysis
go vet ./...

# Format codebase per Go standards
go fmt ./...
```
