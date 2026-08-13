import type { GatewayCommandExecutor } from "../commands/gateway-command-executor.js";
import type { BridgeConfig } from "../config/bridge-config.js";
import {
	parseBridgeAuthChallenge,
	parseBridgeAuthReady,
	parseBridgeCommand,
	parseBridgeServerFrame,
	parseBridgeWelcome,
	type BridgeClientFrame,
	type BridgeServerFrame,
} from "../protocol/bridge-envelope.js";
import type { CommandJournal } from "../wal/command-journal.js";
import type { OutgoingFrameJournal } from "../wal/outgoing-frame-journal.js";

export type BridgeSocketRuntime = {
	profileId: string;
	workspaces: ReadonlyArray<{ workspaceId: string; name: string }>;
	proveRuntimeAuth(challenge: string, signal: AbortSignal): Promise<string>;
	commands: CommandJournal;
	executor: GatewayCommandExecutor;
	outgoing: OutgoingFrameJournal;
};

export async function runBridgeSocket(
	config: BridgeConfig,
	connectionToken: string,
	runtime: BridgeSocketRuntime,
	signal: AbortSignal,
): Promise<void> {
	if (!/^deeix_connection_[A-Za-z0-9_-]+$/.test(connectionToken))
		throw new TypeError("connection token is invalid");
	if (signal.aborted) throw signal.reason ?? new Error("Bridge connection aborted");

	const url = new URL("/api/v1/agent/bridge/connect", config.cloudUrl);
	url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
	const socket = new WebSocket(url, ["deeix.bridge.v1", `deeix.auth.${connectionToken}`]);
	const messages = new SocketMessageQueue(socket);
	const writer = new SocketWriter(socket);
	const abort = () => socket.close(1000, "bridge shutdown");
	signal.addEventListener("abort", abort, { once: true });
	let heartbeat: ReturnType<typeof setInterval> | undefined;
	let unsubscribe = () => {};
	try {
		await waitForOpen(socket, 10_000);
		await writer.send({
			version: 1,
			type: "hello",
			profileId: runtime.profileId,
			ackServerSeq: runtime.commands.contiguousReceipt(),
			ackBridgeSeq: runtime.outgoing.acknowledgedSequence(),
		});
		const challenge = parseBridgeAuthChallenge(
			parseSocketJSON(await messages.next(10_000)),
			runtime.profileId,
		);
		const proof = await runtime.proveRuntimeAuth(challenge.challenge, signal);
		if (!/^[A-Za-z0-9_-]{43}$/.test(proof))
			throw new TypeError("runtime authentication proof is invalid");
		await writer.send({
			version: 1, type: "auth.proof", profileId: runtime.profileId,
			challengeId: challenge.challengeId, proof, workspaces: [...runtime.workspaces],
		});
		parseBridgeAuthReady(
			parseSocketJSON(await messages.next(60_000)),
			runtime.profileId,
		);
		const welcome = parseBridgeWelcome(
			parseSocketJSON(await messages.next(10_000)),
			config.deviceId,
		);
		if ((welcome.ackBridgeSeq ?? 0) > runtime.outgoing.acknowledgedSequence())
			await runtime.outgoing.acknowledge(welcome.ackBridgeSeq ?? 0);
		const pump = new OutgoingFramePump(writer, runtime.outgoing);
		unsubscribe = runtime.outgoing.subscribe(() => void pump.flush().catch(() => socket.close()));
		await pump.flush();

		heartbeat = setInterval(() => {
			if (socket.readyState === WebSocket.OPEN)
				void writer.send({ version: 1, type: "ping" }).catch(() => socket.close());
		}, Math.max(1_000, Math.floor((welcome.heartbeatSeconds * 1_000) / 2)));

		for (;;) {
			if (signal.aborted)
				throw signal.reason ?? new Error("Bridge connection aborted");
			const frame = parseBridgeServerFrame(parseSocketJSON(await messages.next()));
			await handleServerFrame(socket, writer, frame, runtime, signal);
		}
	} finally {
		if (heartbeat !== undefined) clearInterval(heartbeat);
		unsubscribe();
		signal.removeEventListener("abort", abort);
		messages.close();
		if (socket.readyState === WebSocket.OPEN || socket.readyState === WebSocket.CONNECTING)
			socket.close(1000, "bridge reconnect");
	}
}

async function handleServerFrame(
	socket: WebSocket,
	writer: SocketWriter,
	frame: BridgeServerFrame,
	runtime: BridgeSocketRuntime,
	signal: AbortSignal,
): Promise<void> {
	switch (frame.type) {
		case "pong":
			return;
		case "ack.bridge":
			await runtime.outgoing.acknowledge(frame.ackBridgeSeq);
			return;
		case "command": {
			const command = parseBridgeCommand(frame);
			void runtime.executor.dispatch(
				frame.serverSeq,
				frame.commandId,
				command,
				signal,
				async (ackServerSeq) =>
					writer.send({ version: 1, type: "ack.server", ackServerSeq }),
				frame.artifacts,
			).then(
				(outcome) => runtime.outgoing.appendTerminal(
					frame.serverSeq,
					frame.commandId,
					outcome,
				),
			).catch(() => socket.close());
			return;
		}
	}
}

class SocketWriter {
	readonly #socket: WebSocket;
	#queue: Promise<void> = Promise.resolve();

	constructor(socket: WebSocket) {
		this.#socket = socket;
	}

	async send(frame: BridgeClientFrame): Promise<void> {
		const result = this.#queue.then(() => {
			if (this.#socket.readyState !== WebSocket.OPEN)
				throw new Error("Bridge WebSocket is not open");
			this.#socket.send(JSON.stringify(frame));
		});
		this.#queue = result.then(
			() => undefined,
			() => undefined,
		);
		return result;
	}
}

class OutgoingFramePump {
	readonly #writer: SocketWriter;
	readonly #journal: OutgoingFrameJournal;
	#sentThrough = 0;
	#queue: Promise<void> = Promise.resolve();

	constructor(writer: SocketWriter, journal: OutgoingFrameJournal) {
		this.#writer = writer;
		this.#journal = journal;
		this.#sentThrough = journal.acknowledgedSequence();
	}

	async flush(): Promise<void> {
		const result = this.#queue.then(async () => {
			for (const frame of this.#journal.pending(this.#sentThrough)) {
				await this.#writer.send({ version: 1, ...frame });
				this.#sentThrough = frame.bridgeSeq;
			}
		});
		this.#queue = result.then(
			() => undefined,
			() => undefined,
		);
		return result;
	}
}

class SocketMessageQueue {
	readonly #socket: WebSocket;
	readonly #values: unknown[] = [];
	readonly #waiters: Array<{
		resolve: (value: unknown) => void;
		reject: (error: Error) => void;
	}> = [];
	#error: Error | undefined;

	constructor(socket: WebSocket) {
		this.#socket = socket;
		socket.addEventListener("message", this.#onMessage);
		socket.addEventListener("error", this.#onError);
		socket.addEventListener("close", this.#onClose);
	}

	next(timeoutMilliseconds?: number): Promise<unknown> {
		if (this.#values.length > 0) return Promise.resolve(this.#values.shift());
		if (this.#error) return Promise.reject(this.#error);
		return new Promise<unknown>((resolve, reject) => {
			const waiter = { resolve, reject };
			this.#waiters.push(waiter);
			if (timeoutMilliseconds === undefined) return;
			const timeout = setTimeout(() => {
				const index = this.#waiters.indexOf(waiter);
				if (index >= 0) this.#waiters.splice(index, 1);
				reject(new Error("WebSocket message timeout"));
			}, timeoutMilliseconds);
			waiter.resolve = (value) => {
				clearTimeout(timeout);
				resolve(value);
			};
			waiter.reject = (error) => {
				clearTimeout(timeout);
				reject(error);
			};
		});
	}

	close(): void {
		this.#socket.removeEventListener("message", this.#onMessage);
		this.#socket.removeEventListener("error", this.#onError);
		this.#socket.removeEventListener("close", this.#onClose);
		this.#fail(new Error("Bridge message queue closed"));
	}

	readonly #onMessage = (event: MessageEvent): void => {
		const waiter = this.#waiters.shift();
		if (waiter) waiter.resolve(event.data);
		else this.#values.push(event.data);
	};

	readonly #onError = (): void => this.#fail(new Error("WebSocket connection failed"));
	readonly #onClose = (): void => this.#fail(new Error("WebSocket connection closed"));

	#fail(error: Error): void {
		if (this.#error) return;
		this.#error = error;
		for (const waiter of this.#waiters.splice(0)) waiter.reject(error);
	}
}

function waitForOpen(socket: WebSocket, timeoutMilliseconds: number): Promise<void> {
	return new Promise<void>((resolve, reject) => {
		const timeout = setTimeout(() => finish(new Error("WebSocket open timeout")), timeoutMilliseconds);
		const finish = (error?: Error) => {
			clearTimeout(timeout);
			socket.removeEventListener("open", onOpen);
			socket.removeEventListener("error", onError);
			socket.removeEventListener("close", onClose);
			if (error) reject(error);
			else resolve();
		};
		const onOpen = () => finish();
		const onError = () => finish(new Error("WebSocket connection failed"));
		const onClose = () => finish(new Error("WebSocket closed before opening"));
		socket.addEventListener("open", onOpen, { once: true });
		socket.addEventListener("error", onError, { once: true });
		socket.addEventListener("close", onClose, { once: true });
	});
}

function parseSocketJSON(value: unknown): unknown {
	if (typeof value !== "string" || value.length === 0 || value.length > (2 << 20) + (64 << 10))
		throw new TypeError("WebSocket message is invalid");
	try {
		return JSON.parse(value);
	} catch {
		throw new TypeError("WebSocket message is not valid JSON");
	}
}
