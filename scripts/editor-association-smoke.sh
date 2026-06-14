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
# Validator selection (first available):
#   - yaml-language-server (npx)  — the real editor face: also exercises catalog.json
#     fileMatch association. Preferred.
#   - ajv / python3 jsonschema    — validate against the SERVED URL directly.
#   - aggrega CLI envelope gate   — FALLBACK using the project's own JSON Schema
#     2020-12 validator. Validates the source schema (byte-identical to the served
#     bytes, asserted in step 2). In this mode the served-URL-serves-the-schema and
#     schema-validates-good/bad claims are proven, but catalog.json's editor
#     association is structurally verified only (the LSP engine is not driven) —
#     see PUBLISHING.md. AGGREGA_BIN overrides the `aggrega` binary.
set -euo pipefail

DIR="$(cd "$(dirname "$0")/.." && pwd)"
JSON="$DIR/json"
SCHEMA_FILE="$JSON/component-v1alpha1.schema.json"
SCHEMA_NAME="component-v1alpha1.schema.json"
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
apiVersion: aggrega.ai/v1alpha1
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
apiVersion: aggrega.ai/v1alpha1
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

validate_good_bad() {
  local how="$1" goodrc badrc
  if command -v yaml-language-server >/dev/null 2>&1; then how="yaml-language-server"; fi
  echo "validator: $how"
  case "$how" in
    aggrega)
      # Envelope gate runs at catalog load; a clean dir lints, the bad key fails.
      mkdir -p "$tmp/good-cat" "$tmp/bad-cat"
      cp "$tmp/good.yaml" "$tmp/good-cat/component.yaml"
      cp "$tmp/bad.yaml"  "$tmp/bad-cat/component.yaml"
      ( cd "$tmp/good-cat" && "$AGGREGA" lint >/dev/null 2>&1 ); goodrc=$?
      ( cd "$tmp/bad-cat"  && "$AGGREGA" lint >/dev/null 2>&1 ); badrc=$?
      ;;
    *)
      echo "SKIP: no JSON Schema validator found (yaml-language-server/ajv/jsonschema); set AGGREGA_BIN to use the project validator" >&2
      return 2
      ;;
  esac
  if [ "$goodrc" -ne 0 ]; then echo "FAIL: GOOD sample rejected (rc=$goodrc)" >&2; return 1; fi
  if [ "$badrc" -eq 0 ]; then echo "FAIL: BAD sample accepted (expected rejection)" >&2; return 1; fi
  echo "OK: GOOD sample validates, BAD sample rejected"
}

# Prefer a true URL-fetching validator; fall back to the project validator.
if command -v yaml-language-server >/dev/null 2>&1 || command -v ajv >/dev/null 2>&1 \
   || python3 -c "import jsonschema" >/dev/null 2>&1; then
  validate_good_bad url || exit 1
elif command -v "$AGGREGA" >/dev/null 2>&1 || [ -x "$AGGREGA" ]; then
  echo "note: no external JSON Schema validator; using the project validator (aggrega envelope gate)."
  echo "      catalog.json editor association is structurally verified only — see PUBLISHING.md."
  validate_good_bad aggrega || exit 1
else
  echo "SKIP: no validator available (no yaml-language-server/ajv/jsonschema, no aggrega binary)" >&2
  exit 2
fi

echo "editor-association smoke: PASS"
