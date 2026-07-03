#!/usr/bin/env bash
set -euo pipefail

# Minimal MCP stdio smoke test for `spur mcp`. Sends an initialize handshake
# followed by tools/list, resources/list, one resource read, and two
# explain_enum calls (no network needed). The get_ip_context call is run only
# when SPUR_TOKEN points at a real token and SPUR_LIVE=1.
#
# Run it against a built binary from the repo root:
#
#   go build -o spur ./cmd/spur
#   ./scripts/mcp-smoke.sh
#
# Point BIN at a different binary with BIN=/path/to/spur ./scripts/mcp-smoke.sh.

BIN=${BIN:-./spur}
: "${SPUR_TOKEN:=placeholder-for-smoke-only}"
export SPUR_TOKEN

# Keep stdin open after writing the requests so the server has time to
# respond before seeing EOF. Two seconds is ample for offline calls.
send_requests() {
  cat <<'EOF'
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"smoke","version":"0.0.1"},"capabilities":{}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"resources/list"}
{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"spur://schemas/ip-context"}}
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"explain_enum","arguments":{"value":"CALLBACK_PROXY"}}}
{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"explain_enum","arguments":{"value":"NOT_A_REAL_ENUM"}}}
EOF
  if [[ "${SPUR_LIVE:-0}" == "1" ]]; then
    echo '{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"get_api_status","arguments":{}}}'
    echo '{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"get_ip_context","arguments":{"ip":"89.39.106.191"}}}'
  fi
  sleep 2
}

send_requests | "$BIN" mcp
