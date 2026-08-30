import { readdir, readFile } from "node:fs/promises";
import path from "node:path";

const root = process.cwd();
const locales = ["en-US", "zh-CN"];
const sourceRoots = ["app", "components", "entities", "features", "i18n", "shared"];

function namespaceForFile(fileName) {
  return path.basename(fileName, ".json").replace(/-([a-z])/g, (_, letter) => letter.toUpperCase());
}

async function loadCatalog(locale) {
  const directory = path.join(root, "i18n", "messages", locale);
  const files = (await readdir(directory)).filter((file) => file.endsWith(".json")).sort();
  const catalog = {};
  for (const file of files) {
    catalog[namespaceForFile(file)] = JSON.parse(await readFile(path.join(directory, file), "utf8"));
  }
  return catalog;
}

function leafKeys(value, prefix = "") {
  if (typeof value === "string") return [prefix];
  if (!value || typeof value !== "object" || Array.isArray(value)) return [];
  return Object.entries(value).flatMap(([key, child]) => leafKeys(child, prefix ? `${prefix}.${key}` : key));
}

function hasMessage(catalog, messagePath) {
  let current = catalog;
  for (const segment of messagePath.split(".")) {
    if (!current || typeof current !== "object" || !(segment in current)) return false;
    current = current[segment];
  }
  return typeof current === "string";
}

async function sourceFiles(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  const files = [];
  for (const entry of entries) {
    const absolute = path.join(directory, entry.name);
    if (entry.isDirectory()) files.push(...await sourceFiles(absolute));
    else if (/\.(ts|tsx)$/.test(entry.name)) files.push(absolute);
  }
  return files;
}

function translationBindings(source) {
  const bindings = new Map();
  const pattern = /\b(?:const|let)\s+([A-Za-z_$][\w$]*)\s*=\s*(?:await\s+)?(?:useTranslations|getTranslations)\(\s*["']([^"']+)["']\s*\)/g;
  for (const match of source.matchAll(pattern)) {
    const items = bindings.get(match[1]) ?? [];
    items.push({ index: match.index, namespace: match[2], ref: false });
    bindings.set(match[1], items);
  }
  const refPattern = /\bconst\s+([A-Za-z_$][\w$]*)\s*=\s*React\.useRef\(\s*([A-Za-z_$][\w$]*)\s*\)/g;
  for (const match of source.matchAll(refPattern)) {
    const sourceBinding = bindings.get(match[2])?.findLast((item) => item.index <= match.index);
    if (!sourceBinding) continue;
    const items = bindings.get(match[1]) ?? [];
    items.push({ index: match.index, namespace: sourceBinding.namespace, ref: true });
    bindings.set(match[1], items);
  }
  return bindings;
}

function translationCalls(source, bindings) {
  const calls = [];
  for (const [name, declarations] of bindings) {
    const escapedName = name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    const directPattern = new RegExp(`\\b${escapedName}(?:\\.(?:rich|markup|raw))?\\(\\s*(["'])([^"']+)\\1`, "g");
    const refPattern = new RegExp(`\\b${escapedName}\\.current\\(\\s*(["'])([^"']+)\\1`, "g");
    for (const declaration of declarations) {
      const pattern = declaration.ref ? refPattern : directPattern;
      for (const match of source.matchAll(pattern)) {
        if (match.index < declaration.index) continue;
        const nextDeclaration = declarations.find((item) => item.index > declaration.index);
        if (nextDeclaration && match.index >= nextDeclaration.index) continue;
        const line = source.slice(0, match.index).split("\n").length;
        calls.push({ key: `${declaration.namespace}.${match[2]}`, line });
      }
    }
  }
  return calls;
}

const catalogs = Object.fromEntries(await Promise.all(locales.map(async (locale) => [locale, await loadCatalog(locale)])));
const errors = [];
const referenceKeys = new Set(leafKeys(catalogs[locales[0]]));
for (const locale of locales.slice(1)) {
  const localeKeys = new Set(leafKeys(catalogs[locale]));
  for (const key of referenceKeys) if (!localeKeys.has(key)) errors.push(`${locale}: missing catalog key ${key}`);
  for (const key of localeKeys) if (!referenceKeys.has(key)) errors.push(`${locales[0]}: missing catalog key ${key}`);
}

for (const sourceRoot of sourceRoots) {
  for (const file of await sourceFiles(path.join(root, sourceRoot))) {
    const text = await readFile(file, "utf8");
    for (const call of translationCalls(text, translationBindings(text))) {
      for (const locale of locales) {
        if (!hasMessage(catalogs[locale], call.key)) {
          errors.push(`${path.relative(root, file)}:${call.line}: ${locale} missing ${call.key}`);
        }
      }
    }
  }
}

if (errors.length > 0) {
  console.error(errors.join("\n"));
  process.exitCode = 1;
} else {
  console.log(`i18n catalogs complete (${referenceKeys.size} messages)`);
}
