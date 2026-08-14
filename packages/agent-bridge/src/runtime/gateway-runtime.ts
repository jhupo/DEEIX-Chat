import { mkdir } from "node:fs/promises";
import { join } from "node:path";
import { GatewayCommandExecutor } from "../commands/gateway-command-executor.js";
import { SourceRefRegistry } from "../commands/resolve-provider-command.js";
import { readBridgeConfig } from "../config/bridge-config.js";
import { WorkspaceRegistry } from "../config/workspace-registry.js";
import { DeviceIdentity } from "../identity/device-identity.js";
import { CodexAdapter } from "../providers/codex/codex-adapter.js";
import {
	assertCodexVersion,
	startCodexAppServer,
} from "../providers/codex/codex-process.js";
import { ProviderRegistry } from "../providers/provider-registry.js";
import { BridgeCloudClient } from "../transport/cloud-client.js";
import { ArtifactDownloader } from "../transport/artifact-downloader.js";
import { runBridgeSocket } from "../transport/wss-client.js";
import { CommandJournal } from "../wal/command-journal.js";
import { DurableWalStore } from "../wal/durable-wal-store.js";
import { OutgoingFrameJournal } from "../wal/outgoing-frame-journal.js";

export type GatewayRuntimeOptions = {
	dataDirectory: string;
	reconnect?: boolean;
	onConnectionError?: (error: Error) => void;
};

export async function runGateway(
	options: GatewayRuntimeOptions,
	signal: AbortSignal,
): Promise<void> {
	await mkdir(options.dataDirectory, { recursive: true, mode: 0o700 });
	const config = await readBridgeConfig(join(options.dataDirectory, "config.json"));
	const identity = await DeviceIdentity.loadOrCreate(
		join(options.dataDirectory, "device-identity.json"),
	);
	await assertCodexVersion(config.codexExecutable);

	const commandWal = await DurableWalStore.open(
		join(options.dataDirectory, "wal", "commands"),
	);
	const outgoingWal = await DurableWalStore.open(
		join(options.dataDirectory, "wal", "outgoing"),
	);
	const sourceWal = await DurableWalStore.open(
		join(options.dataDirectory, "wal", "sources"),
	);
	const commands = CommandJournal.restore(commandWal);
	const outgoing = OutgoingFrameJournal.restore(outgoingWal);
	const sources = SourceRefRegistry.restore(sourceWal);
	const workspaces = new WorkspaceRegistry();
	for (const workspace of config.workspaces)
		await workspaces.register(workspace.workspaceId, workspace.root);

	const providers = new ProviderRegistry();
	const process = startCodexAppServer(config.codexExecutable);
	const adapter = new CodexAdapter({
		profileId: config.profileId,
		rpc: process.rpc,
		sources,
		closeProcess: process.close,
	});
	providers.register(config.profileId, adapter);
	const executor = new GatewayCommandExecutor(
		commands,
		workspaces,
		sources,
		providers,
		new ArtifactDownloader(config.cloudUrl, workspaces),
	);

	try {
		const manifest = await adapter.start((event) => outgoing.appendEvent(event).then(() => undefined), signal);
		const cloud = new BridgeCloudClient(config.cloudUrl);
		let delay = 1_000;
		for (;;) {
			if (signal.aborted) throw signal.reason ?? new Error("Gateway stopped");
			try {
				const token = await cloud.connectionToken(config, identity);
				await runBridgeSocket(
					config,
					token,
					{
						profileId: config.profileId,
						manifest,
						workspaces: config.workspaces.map(({ workspaceId, name }) => ({ workspaceId, name })),
						proveRuntimeAuth: (challenge, proofSignal) =>
							adapter.proveRuntimeAuth(challenge, proofSignal),
						commands, executor, outgoing,
					},
					signal,
				);
				delay = 1_000;
			} catch (error) {
				if (signal.aborted || options.reconnect === false) throw error;
				options.onConnectionError?.(connectionError(error));
				await wait(delay, signal);
				delay = Math.min(delay * 2, 30_000);
			}
		}
	} finally {
		await providers.close();
		commandWal.close();
		outgoingWal.close();
		sourceWal.close();
	}
}

function connectionError(error: unknown): Error {
	const message = error instanceof Error ? error.message : "Gateway connection failed";
	return new Error(
		[...message]
			.map((character) => character.charCodeAt(0) < 32 ? " " : character)
			.join("")
			.slice(0, 1024),
	);
}

function wait(milliseconds: number, signal: AbortSignal): Promise<void> {
	return new Promise<void>((resolve, reject) => {
		const timeout = setTimeout(done, milliseconds);
		const abort = () => done(signal.reason ?? new Error("Gateway stopped"));
		function done(error?: unknown) {
			clearTimeout(timeout);
			signal.removeEventListener("abort", abort);
			if (error) reject(error);
			else resolve();
		}
		signal.addEventListener("abort", abort, { once: true });
	});
}
