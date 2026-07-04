#!/usr/bin/env bash
set -euo pipefail

# MCP stdio smoke test for `spur mcp`. Drives an initialize handshake, then
# tools/list, resources/list, a resource read, offline tool calls (explain_enum,
# list_feeds), and — only when SPUR_LIVE=1 with a real SPUR_TOKEN — the networked
# tools (get_api_status, get_ip_context batch, feed_status, feed_download,
# export). It captures the server's stdout and ASSERTS the expected content is
# present, so it exits non-zero on a regression instead of just printing.
#
# Run it against a built binary from the repo root:
#
#   go build -o spur ./cmd/spur
#   ./scripts/mcp-smoke.sh
#
# Point BIN at a different binary with BIN=/path/to/spur ./scripts/mcp-smoke.sh.
# Exercise the networked tools with SPUR_LIVE=1 SPUR_TOKEN=… ./scripts/mcp-smoke.sh.

BIN=${BIN:-./spur}
: "${SPUR_TOKEN:=placeholder-for-smoke-only}"
export SPUR_TOKEN

# Every tool the server must advertise in tools/list. Update this list when the
# tool surface changes; the assertion below fails if any is missing.
EXPECTED_TOOLS=(
  get_ip_context
  get_tag_metadata
  get_api_status
  explain_enum
  format_assessment
  list_feeds
  feed_status
  feed_download
  export
)

# Keep stdin open after writing the requests so the server has time to respond
# before seeing EOF. Two seconds is ample for the offline calls.
send_requests() {
  cat <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"smoke","version":"0.0.1"},"capabilities":{}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"resources/list"}
{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"spur://schemas/ip-context"}}
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"explain_enum","arguments":{"value":"CALLBACK_PROXY"}}}
{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"explain_enum","arguments":{"value":"NOT_A_REAL_ENUM"}}}
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"list_feeds","arguments":{}}}
EOF
  local wait=2
  if [[ "${SPUR_LIVE:-0}" == "1" ]]; then
    echo '{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"get_api_status","arguments":{}}}'
    echo '{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"get_ip_context","arguments":{"ips":["89.39.106.191","1.1.1.1"]}}}'
    echo '{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"feed_status","arguments":{"type":"anonymous"}}}'
    echo '{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"feed_download","arguments":{"type":"anonymous"}}}'
    echo '{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"export","arguments":{"feed":"anonymous","max_rows":5}}}'
    # Networked calls take longer; keep stdin open so every response lands
    # before the transport closes (a mid-flight EOF makes the server error).
    wait=20
  fi
  sleep "$wait"
}

# The server exits non-zero if stdin closes while a call is still in flight;
# capture its output regardless so the assertions below are the arbiter.
out=$(send_requests | "$BIN" mcp || true)

echo "$out"

fail=0
assert_contains() {
  # $1 haystack, $2 needle, $3 human description
  if ! grep -qF -- "$2" <<<"$1"; then
    echo "SMOKE FAIL: $3 (missing: $2)" >&2
    fail=1
  fi
}

# The handshake and each listing must have come back.
assert_contains "$out" '"protocolVersion"' "initialize response"
assert_contains "$out" 'spur://schemas/ip-context' "resource read"

# tools/list must advertise every expected tool.
for tool in "${EXPECTED_TOOLS[@]}"; do
  assert_contains "$out" "\"name\":\"$tool\"" "tools/list advertises $tool"
done

# Offline tool calls returned usable content.
assert_contains "$out" 'CALLBACK_PROXY' "explain_enum(CALLBACK_PROXY) result"
assert_contains "$out" 'anonymous-residential' "list_feeds returned the feed catalog"

# Networked tool calls returned their expected shapes.
if [[ "${SPUR_LIVE:-0}" == "1" ]]; then
  assert_contains "$out" '"serviceTier"' "get_api_status result"
  assert_contains "$out" '"results"' "get_ip_context batch results"
  assert_contains "$out" '"download_url"' "feed_download pointer"
  assert_contains "$out" '"row_count"' "export sample"
fi

if [[ $fail -ne 0 ]]; then
  echo "SMOKE FAILED" >&2
  exit 1
fi
echo "SMOKE OK" >&2
