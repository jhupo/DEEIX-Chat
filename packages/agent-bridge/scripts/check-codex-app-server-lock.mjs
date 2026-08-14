import { createHash } from "node:crypto";
import { readFile, readdir } from "node:fs/promises";
import { basename, join } from "node:path";

const [lockPath, tsDirectory, jsonDirectory] = process.argv.slice(2);
if (!lockPath || !tsDirectory || !jsonDirectory) {
	throw new Error("usage: check-codex-app-server-lock <lock> <ts-dir> <json-dir>");
}

const lock = JSON.parse(await readFile(lockPath, "utf8"));
const artifacts = lock.generated_artifacts;
await checkFile(
	join(tsDirectory, "ClientRequest.ts"),
	artifacts["ClientRequest.ts"],
);
await checkFile(
	join(tsDirectory, "ClientNotification.ts"),
	artifacts["ClientNotification.ts"],
);
await checkFile(
	join(tsDirectory, "ServerRequest.ts"),
	artifacts["ServerRequest.ts"],
);
await checkFile(
	join(tsDirectory, "ServerNotification.ts"),
	artifacts["ServerNotification.ts"],
);
await checkFile(
	join(jsonDirectory, "codex_app_server_protocol.schemas.json"),
	artifacts.full_json_bundle,
);
await checkFile(
	join(jsonDirectory, "codex_app_server_protocol.v2.schemas.json"),
	artifacts.v2_json_bundle,
);

for (const [name, expected] of Object.entries(lock.unions)) {
	const source = await readFile(join(tsDirectory, `${name}.ts`), "utf8");
	const actual = [...source.matchAll(/"method": "([^"]+)"/g)].map(
		(match) => match[1],
	);
	const locked = expected.members.map((member) => member.name);
	if (new Set(actual).size !== actual.length || !sameSet(actual, locked)) {
		throw new Error(`${name} union members differ from the schema lock`);
	}
	const canonical = sha256(`${[...actual].sort().join("\n")}\n`);
	if (actual.length !== expected.count || canonical !== expected.canonical_members_sha256) {
		throw new Error(`${name} count or canonical hash differs from the schema lock`);
	}
	if (expected.members.some((member) => !["mapped", "extension", "disabled"].includes(member.disposition))) {
		throw new Error(`${name} has a missing or invalid disposition`);
	}
}

const policyPath = join(
	tsDirectory,
	"..",
	"..",
	"..",
	"src",
	"providers",
	"codex",
	"codex-method-policy.ts",
);
const policy = await readFile(policyPath, "utf8");
checkPolicyObject(policy, "CODEX_SERVER_REQUESTS", lock.unions.ServerRequest);
checkPolicyObject(
	policy,
	"CODEX_SERVER_NOTIFICATIONS",
	lock.unions.ServerNotification,
);
checkDispatchedClientRequests(policy, lock.unions.ClientRequest);

if ((await gitTree(tsDirectory)) !== artifacts.typescript_tree_git_object)
	throw new Error("generated TypeScript tree differs from the schema lock");
if ((await gitTree(jsonDirectory)) !== artifacts.json_tree_git_object)
	throw new Error("generated JSON tree differs from the schema lock");

console.log(`Codex app-server ${lock.upstream.tag} schema lock verified`);

async function checkFile(path, expected) {
	const content = await readCanonicalFile(path);
	if (sha256(content) !== expected.sha256)
		throw new Error(`${basename(path)} SHA-256 differs from the schema lock`);
	if (expected.git_blob && gitObject("blob", content) !== expected.git_blob)
		throw new Error(`${basename(path)} Git blob differs from the schema lock`);
}

async function gitTree(directory) {
	const entries = await readdir(directory, { withFileTypes: true });
	entries.sort((left, right) =>
		Buffer.from(left.name + (left.isDirectory() ? "/" : "")).compare(
			Buffer.from(right.name + (right.isDirectory() ? "/" : "")),
		),
	);
	const parts = [];
	for (const entry of entries) {
		const path = join(directory, entry.name);
		const hash = entry.isDirectory()
			? await gitTree(path)
			: gitObject("blob", await readCanonicalFile(path));
		parts.push(Buffer.from(`${entry.isDirectory() ? "40000" : "100644"} ${entry.name}\0`));
		parts.push(Buffer.from(hash, "hex"));
	}
	return gitObject("tree", Buffer.concat(parts));
}

async function readCanonicalFile(path) {
	return Buffer.from((await readFile(path, "utf8")).replaceAll("\r\n", "\n"));
}

function gitObject(kind, content) {
	return createHash("sha1")
		.update(`${kind} ${content.length}\0`)
		.update(content)
		.digest("hex");
}

function sha256(content) {
	return createHash("sha256").update(content).digest("hex");
}

function sameSet(left, right) {
	return left.length === right.length && left.every((value) => right.includes(value));
}

function checkPolicyObject(source, name, expected) {
	const body = extract(source, `export const ${name} = {`, "} as const");
	const entries = [...body.matchAll(/^\s*(?:"([^"]+)"|([A-Za-z][A-Za-z0-9]*)):\s*"(mapped|extension|disabled)",?\s*$/gm)]
		.map((match) => ({ name: match[1] ?? match[2], disposition: match[3] }));
	if (entries.length !== expected.count)
		throw new Error(`${name} does not cover the locked union`);
	for (const member of expected.members) {
		const actual = entries.find((entry) => entry.name === member.name);
		if (!actual || actual.disposition !== member.disposition)
			throw new Error(`${name} disposition differs for ${member.name}`);
	}
}

function checkDispatchedClientRequests(source, expected) {
	const body = extract(
		source,
		"export const CODEX_DISPATCHED_CLIENT_REQUESTS = [",
		"] as const",
	);
	const methods = [...body.matchAll(/"([^"]+)"/g)].map((match) => match[1]);
	if (methods.length === 0 || new Set(methods).size !== methods.length)
		throw new Error("dispatched ClientRequest registry is empty or duplicated");
	for (const method of methods) {
		const member = expected.members.find((item) => item.name === method);
		if (!member || member.disposition !== "mapped")
			throw new Error(`dispatched ClientRequest is not mapped by the lock: ${method}`);
	}
	const mapped = expected.members
		.filter((member) => member.disposition === "mapped")
		.map((member) => member.name);
	if (!sameSet(methods, mapped))
		throw new Error("mapped ClientRequest methods differ from the dispatched registry");
}

function extract(source, start, end) {
	const from = source.indexOf(start);
	const to = source.indexOf(end, from + start.length);
	if (from < 0 || to < 0) throw new Error(`method policy section is missing: ${start}`);
	return source.slice(from + start.length, to);
}
