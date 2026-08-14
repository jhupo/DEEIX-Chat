#!/usr/bin/env node
import { createHash } from "node:crypto";
import { realpath } from "node:fs/promises";
import { homedir, hostname } from "node:os";
import { basename, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import {
	readBridgeConfig,
	normalizeCloudUrl,
	writeBridgeConfig,
	type BridgeConfig,
} from "./config/bridge-config.js";
import { SourceRefRegistry } from "./commands/resolve-provider-command.js";
import { DeviceIdentity } from "./identity/device-identity.js";
import { CodexAdapter } from "./providers/codex/codex-adapter.js";
import { assertCodexVersion, startCodexAppServer } from "./providers/codex/codex-process.js";
import { runGateway } from "./runtime/gateway-runtime.js";
import { BridgeCloudClient } from "./transport/cloud-client.js";

export async function main(argv = process.argv.slice(2)): Promise<void> {
	const [command, ...args] = argv;
	const options = parseOptions(args);
	const dataDirectory = resolve(
		option(options, "data-dir", join(homedir(), ".deeix-agent-bridge")),
	);
	if (command === "install") {
		const identity = await DeviceIdentity.loadOrCreate(
			join(dataDirectory, "device-identity.json"),
		);
		const server = normalizeCloudUrl(required(options, "server"));
		const userPublicID = required(options, "user");
		if (!/^[a-f0-9]{32}$/.test(userPublicID))
			throw new TypeError("--user must be a DEEIX public user ID");
		const configPath = join(dataDirectory, "config.json");
		const previous = await readOptionalConfig(configPath);
		if (previous && (previous.cloudUrl !== server || previous.userPublicID !== userPublicID))
			throw new Error("existing bridge identity belongs to a different server or user");
		const codexExecutable = option(options, "codex", "codex");
		await assertCodexVersion(codexExecutable);
		const workspaceRoot = await realpath(resolve(required(options, "workspace")));
		const workspaceId = stableOpaqueId("workspace", workspaceRoot);
		const profileId = "codex-default";
		const client = new BridgeCloudClient(server);
		const process = startCodexAppServer(codexExecutable);
		const adapter = new CodexAdapter({
			profileId,
			rpc: process.rpc,
			sources: new SourceRefRegistry(),
			closeProcess: process.close,
		});
		let deviceId: string;
		try {
			await adapter.start(async () => undefined, AbortSignal.timeout(30_000));
			deviceId = await client.enroll(
				userPublicID,
			option(options, "name", hostname()),
			identity,
				(challenge, signal) => adapter.proveRuntimeAuth(challenge, signal),
			);
		} finally {
			await adapter.close();
		}
		const workspaces = previous?.workspaces.filter((item) => item.workspaceId !== workspaceId) ?? [];
		workspaces.push({ workspaceId, root: workspaceRoot, name: basename(workspaceRoot) });
		const config: BridgeConfig = {
			version: 1, cloudUrl: server, userPublicID, deviceId, profileId,
			codexExecutable, workspaces,
		};
		await writeBridgeConfig(configPath, config);
		console.log(`Installed device ${config.deviceId} for ${workspaces.length} workspace(s)`);
		return;
	}
	if (command === "start") {
		await readBridgeConfig(join(dataDirectory, "config.json"));
		const controller = new AbortController();
		const stop = () => controller.abort(new Error("Gateway stopped"));
		process.once("SIGINT", stop);
		process.once("SIGTERM", stop);
		try {
			await runGateway(
				{
					dataDirectory,
					onConnectionError: (error) => console.error(`Gateway connection failed: ${error.message}`),
				},
				controller.signal,
			);
		} finally {
			process.off("SIGINT", stop);
			process.off("SIGTERM", stop);
		}
		return;
	}
	throw new TypeError(
		"usage: deeix-agent-bridge install --server URL --user PUBLIC_ID --workspace ABSOLUTE_PATH [--name NAME] [--codex PATH] [--data-dir DIR]\n" +
			"       deeix-agent-bridge start [--data-dir DIR]",
	);
}

function parseOptions(args: string[]): Map<string, string[]> {
	const result = new Map<string, string[]>();
	for (let index = 0; index < args.length; index += 2) {
		const name = args[index];
		const value = args[index + 1];
		if (!name?.startsWith("--") || !value || value.startsWith("--"))
			throw new TypeError("CLI options must use --name value pairs");
		const key = name.slice(2);
		const values = result.get(key) ?? [];
		values.push(value);
		result.set(key, values);
	}
	return result;
}

function option(
	options: Map<string, string[]>,
	name: string,
	fallback: string,
): string {
	const values = options.get(name);
	if (!values) return fallback;
	if (values.length !== 1) throw new TypeError(`--${name} must be specified once`);
	return values[0] ?? fallback;
}

function required(options: Map<string, string[]>, name: string): string {
	const value = option(options, name, "").trim();
	if (!value) throw new TypeError(`--${name} is required`);
	return value;
}

function stableOpaqueId(prefix: string, value: string): string {
	return `${prefix}-${createHash("sha256").update(value, "utf8").digest("hex").slice(0, 24)}`;
}

async function readOptionalConfig(filePath: string): Promise<BridgeConfig | undefined> {
	try {
		return await readBridgeConfig(filePath);
	} catch (error) {
		if (typeof error === "object" && error !== null && Reflect.get(error, "code") === "ENOENT")
			return undefined;
		throw error;
	}
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? "").href) {
	main().catch((error) => {
		console.error(error instanceof Error ? error.message : "Gateway failed");
		process.exitCode = 1;
	});
}
