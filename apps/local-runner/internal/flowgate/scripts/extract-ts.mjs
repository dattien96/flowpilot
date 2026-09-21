// CP-67 P-2b (Task-383 B-11): TypeScript/React symbol extractor.
//
// Usage: node extract-ts.mjs <source-file>
// Emits one JSON object on stdout:
//   {"symbols":[{"name","kind","signature","startLine","endLine"}]}
// kind ∈ function|method|interface|struct|class (class covers TS classes;
// React components are plain functions). Lines are 1-based inclusive.
//
// Requires the workspace's own node_modules to resolve `typescript`
// (a React/TS repo always has it — CP-67 Q-1 assumption). Exit 3 with a
// diagnostic when it cannot load, so the Go adapter falls back cleanly.

import { createRequire } from "node:module";

const file = process.argv[2];
if (!file) {
  console.error("usage: node extract-ts.mjs <source-file>");
  process.exit(2);
}

let ts;
try {
  ts = createRequire(import.meta.url)("typescript");
} catch (err) {
  console.error("typescript module not resolvable from workspace: " + err.message);
  process.exit(3);
}

import { readFileSync } from "node:fs";

const sourceText = readFileSync(file, "utf8");
const sf = ts.createSourceFile(file, sourceText, ts.ScriptTarget.Latest, /*setParentNodes*/ true, file.endsWith(".tsx") || file.endsWith(".jsx") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);

const symbols = [];

function lineOf(pos) {
  const { line } = sf.getLineAndCharacterOfPosition(pos);
  return line + 1; // 1-based
}

function signatureOf(node, name) {
  const text = node.getText(sf);
  // Signature = declaration up to the first body-opening brace or arrow body.
  const brace = text.indexOf("{");
  const arrow = text.indexOf("=>");
  let cut = text.length;
  if (brace >= 0) cut = brace;
  if (arrow >= 0 && (arrow < cut)) cut = arrow;
  return text.slice(0, cut).replace(/\s+/g, " ").trim();
}

function push(node, name, kind) {
  symbols.push({
    name,
    kind,
    signature: signatureOf(node, name),
    startLine: lineOf(node.getStart(sf)),
    endLine: lineOf(node.getEnd()),
  });
}

function walk(node) {
  switch (node.kind) {
    case ts.SyntaxKind.FunctionDeclaration:
      if (node.name) push(node, node.name.text, "function");
      break;
    case ts.SyntaxKind.MethodDeclaration:
      push(node, node.name.getText(sf), "method");
      break;
    case ts.SyntaxKind.ClassDeclaration:
      if (node.name) push(node, node.name.text, "class");
      break;
    case ts.SyntaxKind.InterfaceDeclaration:
      if (node.name) push(node, node.name.text, "interface");
      break;
    case ts.SyntaxKind.VariableDeclaration: {
      const init = node.initializer;
      if (init && (ts.isArrowFunction(init) || ts.isFunctionExpression(init))) {
        push(node, node.name.getText(sf), "function");
      } else if (init && ts.isObjectLiteralExpression(init) === false && !ts.isLiteralExpression(init) && node.name.getText(sf).match(/^[a-zA-Z_$][\w$]*$/)) {
        // Top-level const with a computed initializer is a hiding spot; the
        // Go side validates its body text against the whitelist.
        push(node, node.name.getText(sf), "function");
      }
      break;
    }
    case ts.SyntaxKind.PropertyDeclaration: {
      const init = node.initializer;
      if (init && (ts.isArrowFunction(init) || ts.isFunctionExpression(init))) {
        push(node, node.name.getText(sf), "method");
      }
      break;
    }
    default:
      break;
  }
  ts.forEachChild(node, walk);
}

walk(sf);

process.stdout.write(JSON.stringify({ symbols }, null, 0));
