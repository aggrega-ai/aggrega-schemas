#!/usr/bin/env node
// LSP editor-association smoke.
//
// Drives the real yaml-language-server LANGUAGE SERVICE — the engine VS Code and
// other editors use for YAML schema association — against this repo's
// editor-association catalog (catalog.json). It proves the fileMatch -> url
// association end to end, which the served-URL smoke (editor-association-smoke.sh)
// does not exercise:
//
//   GOOD     a valid user-authored entity (component.yaml) is ASSOCIATED by
//            fileMatch and validates clean (zero diagnostics);
//   BAD      the same kind with a schema violation is REJECTED with a diagnostic
//            whose range points at the offending field;
//   EXCLUDED a file representing a machine-schema kind (excluded from catalog.json)
//            is NOT associated — the same service returns zero diagnostics, while
//            it flags BAD, so the empty result is non-association, not an inert
//            validator. A contrast check places the same content at a matching
//            path and confirms the engine WOULD flag it once associated.
//
// The schemas are served from the hub at https://hub.aggrega.ai/schemas/<filename>
// (served as-is from json/, keyed by filename); aggrega.ai/schemas/<name> is the
// schema's $id identity only and is never served. This smoke serves json/ locally
// by FILENAME — standing in for the hub static path — and rewrites catalog.json's
// hub urls to the local server IN MEMORY ONLY (the committed catalog.json urls are
// never edited). The schemaRequestService rewrites any host (hub or aggrega.ai $id
// base) to the local server by basename, so cross-document $ref resolves locally.
//
// Version pin: yaml-language-server@1.23.0 — installed into a throwaway prefix
// (nothing committed; no node_modules / package.json land in this repo). If it
// cannot be installed/run, the smoke exits 2 with an explicit re-defer message and
// never fakes the proof or downgrades to the served-URL / envelope-gate floor.

import { createRequire } from 'node:module';
import { execFileSync } from 'node:child_process';
import { mkdtempSync, readFileSync, existsSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, basename } from 'node:path';
import { fileURLToPath } from 'node:url';
import http from 'node:http';

const YLS_VERSION = '1.23.0';

const SCRIPT_DIR = fileURLToPath(new URL('.', import.meta.url));
const REPO = join(SCRIPT_DIR, '..');
const JSON_DIR = join(REPO, 'json');
const CATALOG = join(REPO, 'catalog.json');

function die(code, msg) {
  console.error(msg);
  process.exit(code);
}

// 1. Ensure the pinned yaml-language-server is importable (throwaway prefix).
function loadYls() {
  const prefix = mkdtempSync(join(tmpdir(), 'yls-'));
  try {
    execFileSync('npm', ['install', '--no-save', '--prefix', prefix, `yaml-language-server@${YLS_VERSION}`], {
      stdio: ['ignore', 'ignore', 'pipe'],
    });
  } catch (e) {
    die(2, `SKIP: yaml-language-server@${YLS_VERSION} could not be installed — re-defer to CI.\n${e.stderr || e}`);
  }
  const entry = join(prefix, 'node_modules', 'yaml-language-server');
  if (!existsSync(entry)) die(2, `SKIP: yaml-language-server not present after install — re-defer to CI.`);
  const req = createRequire(import.meta.url);
  const mod = req(entry);
  const resolved = req(join(entry, 'package.json')).version;
  return { mod, resolved };
}

// 2. Serve json/ locally by basename (stand-in for the hub static path).
function serveJson() {
  const server = http.createServer((q, s) => {
    const name = basename(decodeURIComponent(q.url.split('?')[0]));
    const file = join(JSON_DIR, name);
    if (!existsSync(file)) { s.statusCode = 404; s.end('not found'); return; }
    s.setHeader('content-type', 'application/json');
    s.end(readFileSync(file));
  });
  return new Promise((resolve) => server.listen(0, '127.0.0.1', () => {
    resolve({ server, port: server.address().port });
  }));
}

// Rewrite any schema uri (hub host, or the aggrega.ai $id base, incl. internal/)
// to the local server by basename.
function toLocal(uri, port) {
  return `http://127.0.0.1:${port}/${basename(new URL(uri).pathname)}`;
}

async function main() {
  const { mod: yls, resolved } = loadYls();
  const { getLanguageService, TextDocument } = yls;
  const { server, port } = await serveJson();

  // 3. Build the association map from the COMMITTED catalog.json, host-rewritten
  //    to the local server in memory. The committed urls are not touched.
  const catalog = JSON.parse(readFileSync(CATALOG, 'utf8'));
  const schemas = catalog.schemas.map((e) => ({
    uri: toLocal(e.url, port),
    fileMatch: e.fileMatch,
    priority: 3, // Settings priority — wins over any built-in store
  }));

  const schemaRequestService = async (uri) => {
    const local = uri.startsWith('http://127.0.0.1:') ? uri : toLocal(uri, port);
    const r = await fetch(local);
    if (!r.ok) throw new Error(`schema fetch ${local} -> ${r.status}`);
    return await r.text();
  };
  const workspaceContext = {
    resolveRelativePath: (rel, base) => new URL(rel, base).href,
  };

  const ls = getLanguageService({ schemaRequestService, workspaceContext, telemetry: null });
  ls.configure({ validate: true, completion: false, hover: false, schemas });

  const doc = (uri, body) => TextDocument.create(uri, 'yaml', 1, body);
  const validate = (uri, body) => ls.doValidation(doc(uri, body), false);

  // 4. Samples.
  const GOOD = `apiVersion: aggrega.ai/v1beta1
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
`;
  const BAD = `apiVersion: aggrega.ai/v1beta1
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
`;
  // An excluded machine-schema kind (template-context) — catalog.json has no entry
  // for it, and no user-kind fileMatch glob matches this path.
  const EXCLUDED = `someInternalField: 1
shape: not-a-user-entity
`;

  const goodUri = 'file:///ws/component/demo/1.0.0/component.yaml';   // matches **/component.yaml
  const badUri = 'file:///ws/component/broken/1.0.0/component.yaml';  // matches **/component.yaml
  const exclUri = 'file:///ws/template-context.yaml';                 // matches no catalog fileMatch
  const exclContrastUri = 'file:///ws/component/contrast/1.0.0/component.yaml'; // force-associate the excluded body

  const goodDiag = await validate(goodUri, GOOD);
  const badDiag = await validate(badUri, BAD);
  const exclDiag = await validate(exclUri, EXCLUDED);
  const exclContrastDiag = await validate(exclContrastUri, EXCLUDED);

  server.close();

  // 5. Assertions. Schema-association diagnostics from yaml-language-server are
  //    reported at Warning severity, so the rejection signal is "any diagnostic
  //    from the associated schema", not Error-only.
  const fails = [];
  if (goodDiag.length !== 0) fails.push(`GOOD produced ${goodDiag.length} diagnostic(s): ${JSON.stringify(goodDiag)}`);
  if (badDiag.length < 1) fails.push(`BAD produced no diagnostic (expected a rejection)`);
  if (exclDiag.length !== 0) fails.push(`EXCLUDED produced ${exclDiag.length} diagnostic(s) — it should not associate: ${JSON.stringify(exclDiag)}`);
  if (exclContrastDiag.length < 1) fails.push(`EXCLUDED-CONTRAST produced no diagnostic — engine-liveness check failed: ${JSON.stringify(exclContrastDiag)}`);

  const summary = {
    yamlLanguageServer: resolved,
    pinned: YLS_VERSION,
    servedFrom: `http://127.0.0.1:<port>/<filename>  (json/, ${catalog.schemas.length} catalog entries)`,
    results: {
      good: { uri: goodUri, diagnostics: goodDiag.length, associated: true },
      bad: {
        uri: badUri,
        diagnostics: badDiag.length,
        firstMessage: badDiag[0]?.message,
        firstSeverity: badDiag[0]?.severity,
        firstRange: badDiag[0]?.range,
        all: badDiag.map((d) => d.message),
      },
      excluded: { uri: exclUri, diagnostics: exclDiag.length, associated: false },
      excludedContrast: { uri: exclContrastUri, diagnostics: exclContrastDiag.length, all: exclContrastDiag.map((d) => d.message) },
    },
  };
  console.log(JSON.stringify(summary, null, 2));

  if (fails.length) {
    die(1, 'FAIL: lsp-association-smoke\n  - ' + fails.join('\n  - '));
  }
  console.log('lsp-association-smoke: PASS');
}

main().catch((e) => die(1, `FAIL: ${e.stack || e}`));
