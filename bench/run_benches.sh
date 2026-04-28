#!/usr/bin/env bash
# run_benches.sh — runs the claw-code-go performance bench suite, prints a
# markdown-formatted result table to stdout, and saves the raw `go test` output
# under bench/results/ for archival.
#
# Usage:
#   ./bench/run_benches.sh                # default benchtime 2s
#   BENCHTIME=5s ./bench/run_benches.sh   # override
#
# Requirements: a Go toolchain on PATH. If you use the iterion devbox shell,
# this script is happy with whatever `go` it finds.

set -euo pipefail

# Resolve the repo root regardless of where the script is invoked from.
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." &>/dev/null && pwd)"

BENCHTIME="${BENCHTIME:-2s}"
RESULTS_DIR="${SCRIPT_DIR}/results"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
RAW_FILE="${RESULTS_DIR}/bench_${TIMESTAMP}.txt"

mkdir -p "${RESULTS_DIR}"

cd "${REPO_ROOT}"

echo "Running benchmarks (benchtime=${BENCHTIME}); raw output → ${RAW_FILE}" >&2
go test -bench=. -benchmem -benchtime="${BENCHTIME}" -run='^$' ./internal/api/... \
    | tee "${RAW_FILE}"

# Emit a markdown-formatted summary table from the raw output. The standard
# `go test -bench` line shape is:
#   Benchmark<Name>-<GOMAXPROCS>   <iters>   <ns/op> ns/op   <B/op> B/op   <allocs/op> allocs/op
echo
echo "## Benchmark results — ${TIMESTAMP}"
echo
echo "| Benchmark | iters | ns/op | B/op | allocs/op |"
echo "|---|---:|---:|---:|---:|"
awk '
    /^Benchmark/ {
        name = $1
        sub(/-[0-9]+$/, "", name)
        iters = $2
        ns_op = "-"; b_op = "-"; allocs_op = "-"
        for (i = 3; i <= NF; i++) {
            if ($(i+1) == "ns/op")     ns_op    = $i
            if ($(i+1) == "B/op")      b_op     = $i
            if ($(i+1) == "allocs/op") allocs_op = $i
        }
        printf("| %s | %s | %s | %s | %s |\n", name, iters, ns_op, b_op, allocs_op)
    }
' "${RAW_FILE}"
