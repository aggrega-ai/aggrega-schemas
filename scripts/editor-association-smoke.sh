#!/usr/bin/env bash
# Editor-association smoke for the published schemas.
#
# Proves the served-by-URL path end to end for a representative authored kind
# (Component). The schemas are served from the hub at
# https://hub.aggrega.ai/schemas/<filename> (served as-is from json/, keyed by
# filename); aggrega.ai/schemas/<name> is the schema's $id identity only and is
# never served, so this smoke serves json/ locally by FILENAME — standing in for
# the hub static path — and does NOT assert anything about the $id.
#   1. serve json/ over local HTTP by filename (stand-in for the hub static path);
#   2. fetch the served schema URL and assert it is byte-identical to the source
#      file (served-as-is verified);
#   3. validate a known-GOOD and a known-BAD sample entity against the schema, the
#      GOOD one passing and the BAD one failing.
#
# Association / validation layer (preferred first):
#   - yaml-language-server (the real editor face) via scripts/lsp-association-smoke.mjs
#     — drives the language service's catalog.json fileMatch association on good /
#     bad / excluded samples. Used when `node` is available. This is the engine an
#     editor actually runs.
#   - aggrega CLI envelope gate — FALLBACK when node is unavailable, using the
#     project's own JSON Schema 2020-12 validator on the source schema (byte-identical
#     to the served bytes, asserted in step 2). In this mode the served-URL and
#     schema-validates-good/bad claims are proven, but catalog.json's editor
#     association is structurally verified only (the LSP engine is not driven) —
#     see PUBLISHING.md. AGGREGA_BIN overrides the `aggrega` binary.
set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"
JSON="$DIR/json"
SCHEMA_FILE="$JSON/component-v1beta1.schema.json"
SCHEMA_NAME="component-v1beta1.schema.json"
PORT="${SMOKE_PORT:-8731}"
AGGREGA="${AGGREGA_BIN:-aggrega}"

tmp="$(mktemp -d)"
cleanup() { [ -n "${SRV_PID:-}" ] && kill "$SRV_PID" 2>/dev/null || true; rm -rf "$tmp"; }
trap cleanup EXIT

# 1. serve json/ locally — python3 http.server if it has stdlib http, else node
#    (built-in http, no npx). A static file server over json/ is all we need.
if python3 -c "import http.server" >/dev/null 2>&1; then
  ( cd "$JSON" && python3 -m http.server "$PORT" >/dev/null 2>&1 ) &
  SRV_PID=$!
elif command -v node >/dev/null 2>&1; then
  node -e 'const http=require("http"),fs=require("fs"),pth=require("path");const root=process.argv[1],port=+process.argv[2];http.createServer((q,s)=>{const f=pth.join(root,decodeURIComponent(q.url.split("?")[0]));fs.readFile(f,(e,d)=>{if(e){s.statusCode=404;s.end("not found")}else{s.end(d)}})}).listen(port,"127.0.0.1")' "$JSON" "$PORT" >/dev/null 2>&1 &
  SRV_PID=$!
else
  echo "SKIP: no local static server (python3 http.server or node) available" >&2
  exit 2
fi
for _ in $(seq 1 30); do curl -fsS -o /dev/null "http://127.0.0.1:$PORT/$SCHEMA_NAME" 2>/dev/null && break; sleep 0.2; done

# 2. served-as-is: fetched bytes byte-identical to the source file
url="http://127.0.0.1:$PORT/$SCHEMA_NAME"
curl -fsS "$url" -o "$tmp/served.json" || { echo "FAIL: served URL did not resolve: $url" >&2; exit 1; }
if ! diff -q "$tmp/served.json" "$SCHEMA_FILE" >/dev/null; then
  echo "FAIL: served bytes differ from source $SCHEMA_FILE" >&2; exit 1
fi
echo "OK: $url serves the schema byte-identical to source"

# good / bad samples (BAD carries an unknown spec key the envelope forbids)
cat > "$tmp/good.yaml" <<'YAML'
apiVersion: aggrega.ai/v1beta1
kind: Component
metadata:
  name: demo/widget
  version: "1.0.0"
  labels:
    component.aggrega.ai/type: "server"
spec:
  schema:
    values:
      size:
        type: string
        default: "small"
YAML
cat > "$tmp/bad.yaml" <<'YAML'
apiVersion: aggrega.ai/v1beta1
kind: Component
metadata:
  name: demo/widget
  version: "1.0.0"
  labels:
    component.aggrega.ai/type: "server"
spec:
  schema:
    values:
      size:
        type: string
  bogusUnknownKey: nope
YAML

# Project-validator fallback (envelope gate). Used only when node is unavailable;
# proves good/bad validation against the source schema but does NOT drive the LSP
# association engine — catalog.json association is structurally verified only.
validate_envelope_floor() {
  local goodrc badrc
  echo "validator: aggrega envelope gate (association structurally verified only — see PUBLISHING.md)"
  mkdir -p "$tmp/good-cat" "$tmp/bad-cat"
  cp "$tmp/good.yaml" "$tmp/good-cat/component.yaml"
  cp "$tmp/bad.yaml"  "$tmp/bad-cat/component.yaml"
  ( cd "$tmp/good-cat" && "$AGGREGA" lint >/dev/null 2>&1 ); goodrc=$?
  ( cd "$tmp/bad-cat"  && "$AGGREGA" lint >/dev/null 2>&1 ); badrc=$?
  if [ "$goodrc" -ne 0 ]; then echo "FAIL: GOOD sample rejected (rc=$goodrc)" >&2; return 1; fi
  if [ "$badrc" -eq 0 ]; then echo "FAIL: BAD sample accepted (expected rejection)" >&2; return 1; fi
  echo "OK: GOOD sample validates, BAD sample rejected"
}

# Drive the real association engine when node is available; the .mjs serves json/
# and runs yaml-language-server's catalog.json association on good/bad/excluded
# samples. Fall back to the envelope floor only when node is absent.
if command -v node >/dev/null 2>&1; then
  echo "association engine: yaml-language-server via scripts/lsp-association-smoke.mjs"
  node "$DIR/scripts/lsp-association-smoke.mjs"; rc=$?
  case "$rc" in
    0) : ;;  # engine smoke passed
    2) echo "note: LSP engine could not run (re-defer to CI — see message above)."
       if command -v "$AGGREGA" >/dev/null 2>&1 || [ -x "$AGGREGA" ]; then validate_envelope_floor || exit 1; fi ;;
    *) echo "FAIL: LSP association engine smoke failed" >&2; exit 1 ;;
  esac
elif command -v "$AGGREGA" >/dev/null 2>&1 || [ -x "$AGGREGA" ]; then
  echo "note: node unavailable; using the project validator."
  validate_envelope_floor || exit 1
else
  echo "SKIP: no association engine (node) and no aggrega binary available" >&2
  exit 2
fi

echo "editor-association smoke: PASS"
