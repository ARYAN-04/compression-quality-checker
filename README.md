# can-i-compress-it

A command-line benchmarking harness that measures how MCP tool-schema
compression impacts tool-use accuracy in locally deployed language models.

It executes a full factorial evaluation matrix (tasks x compression levels)
against any OpenAI-compatible endpoint, scores every tool call with
BFCL-style structural validation, computes paired-bootstrap confidence
intervals, detects per-model "failure cliffs", and emits model-size-specific
schema-compression safety guidance as JSON, Markdown, terminal tables, and
standalone responsive HTML reports.

## Why

MCP servers routinely consume 10k-18k tokens of JSON schema before an agent
reasons a single step. Compressors like Atlassian's `mcp-compressor` cut that
by 70-97% — but their accuracy claims are validated on frontier-scale models.
Small local models (0.3B-14B) are known to be far more sensitive to schema
representation. This harness finds the exact compression level where a given
model starts failing, empirically.

## Install

Requires Go 1.27+.

```bash
# Build binary in the project directory
go build -o can-i-compress-it ./cmd/can-i-compress-it

# Or install to $GOPATH/bin
go install ./cmd/can-i-compress-it
```

## Quick start

```bash
# 1. Synthesize an intent-based task suite with 30 distractor tools
can-i-compress-it suite generate -n 30 --ambiguity high --prompt-style intent \
  --distractors 30 --seed 42 -o ./suites/synthetic_v1.json

# 2. Offline pipeline verification with parameter count recording
can-i-compress-it run --model qwen-3.5-0.8b --params-b 0.8 --mock \
  --suite ./suites/synthetic_v1.json \
  --output ./results/qwen_results.json \
  --html-report ./results/qwen_report.html \
  --md-table ./results/safety_matrix.md

# 3. Real sweep against a local Ollama model
can-i-compress-it run \
  --model qwen3.5:0.8b \
  --params-b 0.8 \
  --endpoint http://localhost:11434/v1 \
  --suite ./suites/synthetic_v1.json \
  --output ./results/qwen_results.json \
  --html-report ./results/qwen_report.html

# 4. Import a live MCP server with pre-annotated benchmark templates
can-i-compress-it suite import --template sqlite -o ./suites/sqlite.json \
  -- npx -y @modelcontextprotocol/server-sqlite /tmp/test.db

# 5. Multi-trace analysis and cross-model regression across all saved runs
can-i-compress-it analyze -i "results/*.json" \
  --bootstrap-iterations 1000 --output-table ./results/combined_safety.md
```

If `--suite` points at the default path and no file exists, an embedded
synthetic suite is used so `run` works out of the box.

## Commands

| Command | Purpose |
|---|---|
| `run` | Full factorial matrix against one model endpoint |
| `test` | Isolated single-prompt probe at one compression level |
| `suite generate` | Synthesize task suites with parameterized ambiguity and distractors |
| `suite import` | Ingest tool schemas from a live MCP server (with template matching) |
| `analyze` | Bootstrap intervals, failure cliffs, and regression from single or glob traces |
| `report` | Re-render traces as Markdown/HTML without recomputation |

Run any command with `-h` for full flag documentation.

## Compression levels

| Level | Schema representation | Token savings |
|---|---|---|
| `none` | Full uncompressed JSON schema | 0% (baseline) |
| `low` | Minor description trimming; params intact | 15-30% |
| `medium` | Shortened descriptions; optional params summarized | 35-55% |
| `high` | Names/types only; all descriptive text stripped | 60-80% |
| `max` | Two-step meta-tools (`get_tool_schema`, `invoke_tool`) | 70-97% |

By default the harness applies these levels with its built-in deterministic
compressor. Pass `--compressor /path/to/mcp-compressor` to wrap the real
production binary instead.

## What it measures

- **Tool-selection accuracy** — exact-name match of the invoked tool.
- **Parameter-fill accuracy** — structural (AST-normalized) comparison of
  argument payloads: types, required keys, enums; key order and whitespace
  ignored.
- **Multi-step completion rate** — every hop of a chained task correct and in
  dependency order, including threaded values (`session_id`-style).
- **Failure cliff** — first level where accuracy drops >=10pp below the
  model's own uncompressed baseline, with paired-bootstrap 95% CIs.
- **Scale regression** — fits cliff depth vs log(parameter count) across
  subject models using recorded `--params-b` metadata.

## Architecture

```
cmd/can-i-compress-it    entry point
internal/cmd             cobra routing (run/test/suite/analyze/report)
internal/domain          shared types, interfaces, level parsing
internal/suite           generator, distractor padding, loader, server templates
internal/mcp             subprocess proxy, builtin compressor, meta-tools, stdio client
internal/inference       OpenAI-compatible client + deterministic mock
internal/eval            BFCL-style matcher + multi-hop chain verifier
internal/runner          factorial scheduler / agent loop
internal/stats           paired bootstrap, cliffs, log regression
internal/report          JSON / Markdown / HTML / terminal renderers
```

## Development

```bash
go test ./...                        # unit tests
go vet ./...                         # static analysis
go run mvdan.cc/gofumpt@latest -w .  # format
```

Generated artifacts (`results/`, `suites/`) are not committed.

## Status

v0.2.0 — all roadmap phases implemented and verified. Supports distractor
catalogue scaling, natural-intent prompting, live MCP server schema import
with templates, and multi-model trace aggregation with log regression.
See `plan.html` (local) for the full specification.
