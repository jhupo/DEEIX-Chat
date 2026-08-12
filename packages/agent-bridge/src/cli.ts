#!/usr/bin/env node
import { homedir, hostname } from "node:os";
import { join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import {
	readBridgeConfig,
	writeBridgeConfig,
} from "./config/bridge-config.js";
import { DeviceIdentity } from "./identity/device-identity.js";
import { runGateway } from "./runtime/gateway-runtime.js";
import { BridgeCloudClient } from "./transport/cloud-client.js";

export async function main(argv = process.argv.slice(2)): Promise<void> {
	const [command, ...args] = argv;
	const options = parseOptions(args);
	const dataDirectory = resolve(
		option(options, "data-dir", join(homedir(), ".deeix-agent-bridge")),
	);
	if (command === "pair") {
		const identity = await DeviceIdentity.loadOrCreate(
			join(dataDirectory, "device-identity.json"),
		);
		const server = required(options, "server");
		const enrollmentCode = required(options, "code");
		const client = new BridgeCloudClient(server);
		const config = await client.pair(
			enrollmentCode,
			option(options, "name", hostname()),
			identity,
		);
		await writeBridgeConfig(join(dataDirectory, "config.json"), config);
		console.log(`Paired device ${config.deviceId}`);
		return;
	}
	if (command === "run") {
		await readBridgeConfig(join(dataDirectory, "config.json"));
		const controller = new AbortController();
		const stop = () => controller.abort(new Error("Gateway stopped"));
		process.once("SIGINT", stop);
		process.once("SIGTERM", stop);
		try {
			await runGateway(
				{
					dataDirectory,
					profileId: required(options, "profile"),
					codexExecutable: option(options, "codex", "codex"),
					workspaces: workspaces(options),
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
		"usage: deeix-agent-bridge pair --server URL --code CODE [--name NAME] [--data-dir DIR]\n" +
			"       deeix-agent-bridge run --profile PROFILE --workspace ID=ABSOLUTE_PATH [--workspace ...] [--codex PATH] [--data-dir DIR]",
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

function workspaces(options: Map<string, string[]>): Array<{
	workspaceId: string;
	root: string;
}> {
	const values = options.get("workspace") ?? [];
	if (values.length === 0) throw new TypeError("--workspace is required");
	return values.map((value) => {
		const separator = value.indexOf("=");
		if (separator <= 0 || separator === value.length - 1)
			throw new TypeError("--workspace must be ID=ABSOLUTE_PATH");
		return {
			workspaceId: value.slice(0, separator),
			root: value.slice(separator + 1),
		};
	});
}

if (import.meta.url === pathToFileURL(process.argv[1] ?? "").href) {
	main().catch((error) => {
		console.error(error instanceof Error ? error.message : "Gateway failed");
		process.exitCode = 1;
	});
}
