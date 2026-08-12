import { once } from "node:events";
import type { Readable, Writable } from "node:stream";

type RpcId = number | string;

export type RpcError = { code: number; message: string; data?: unknown };
export type RpcNotification = { method: string; params?: unknown };
export type RpcServerRequest = { id: RpcId; method: string; params?: unknown };

export type RpcClientOptions = {
	maxLineBytes?: number;
	onNotification?: (notification: RpcNotification) => void | Promise<void>;
	onServerRequest?: (request: RpcServerRequest) => unknown | Promise<unknown>;
};

type PendingRequest = {
	resolve: (value: unknown) => void;
	reject: (reason: Error) => void;
	cleanup: () => void;
};

export class JsonLineRpcClient {
	readonly #input: Writable;
	readonly #output: Readable;
	readonly #maxLineBytes: number;
	#onNotification?: RpcClientOptions["onNotification"];
	#onServerRequest?: RpcClientOptions["onServerRequest"];
	readonly #pending = new Map<number, PendingRequest>();
	readonly #dataListener: (chunk: Buffer | string) => void;
	#buffer = Buffer.alloc(0);
	#nextId = 1;
	#notificationQueue: Promise<void> = Promise.resolve();
	#closedError: Error | undefined;

	constructor(
		input: Writable,
		output: Readable,
		options: RpcClientOptions = {},
	) {
		this.#input = input;
		this.#output = output;
		this.#maxLineBytes = options.maxLineBytes ?? 4 * 1024 * 1024;
		this.#onNotification = options.onNotification;
		this.#onServerRequest = options.onServerRequest;
		if (this.#maxLineBytes < 1024)
			throw new TypeError("maxLineBytes must be at least 1024");

		this.#dataListener = (chunk) =>
			this.#consume(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
		output.on("data", this.#dataListener);
		output.once("end", () => this.close(new Error("app-server output ended")));
		output.once("error", (error) => this.close(asError(error)));
		input.once("error", (error) => this.close(asError(error)));
	}

	async request<T>(
		method: string,
		params?: unknown,
		signal?: AbortSignal,
	): Promise<T> {
		this.#assertOpen();
		validateMethod(method);
		if (signal?.aborted) throw signal.reason ?? new Error("request aborted");
		const id = this.#nextId++;
		let abort: (() => void) | undefined;
		const result = new Promise<T>((resolve, reject) => {
			const cleanup = () => {
				if (abort) signal?.removeEventListener("abort", abort);
			};
			abort = () => {
				if (!this.#pending.delete(id)) return;
				cleanup();
				reject(asError(signal?.reason ?? new Error("request aborted")));
			};
			signal?.addEventListener("abort", abort, { once: true });
			this.#pending.set(id, {
				resolve: resolve as (value: unknown) => void,
				reject,
				cleanup,
			});
		});

		try {
			await this.#write(
				params === undefined ? { id, method } : { id, method, params },
			);
		} catch (error) {
			const pending = this.#pending.get(id);
			this.#pending.delete(id);
			pending?.cleanup();
			pending?.reject(asError(error));
		}
		return result;
	}

	async notify(method: string, params?: unknown): Promise<void> {
		this.#assertOpen();
		validateMethod(method);
		await this.#write(params === undefined ? { method } : { method, params });
	}

	setHandlers(options: Pick<RpcClientOptions, "onNotification" | "onServerRequest">): void {
		this.#assertOpen();
		this.#onNotification = options.onNotification;
		this.#onServerRequest = options.onServerRequest;
	}

	close(reason: Error = new Error("RPC client closed")): void {
		if (this.#closedError) return;
		this.#closedError = reason;
		this.#output.off("data", this.#dataListener);
		for (const pending of this.#pending.values()) {
			pending.cleanup();
			pending.reject(reason);
		}
		this.#pending.clear();
	}

	#consume(chunk: Buffer): void {
		if (this.#closedError) return;
		this.#buffer = Buffer.concat([this.#buffer, chunk]);
		if (
			this.#buffer.length > this.#maxLineBytes &&
			this.#buffer.indexOf(0x0a) < 0
		) {
			this.close(
				new RangeError("app-server frame exceeds the configured limit"),
			);
			return;
		}

		let newline = this.#buffer.indexOf(0x0a);
		while (newline >= 0) {
			const line = this.#buffer.subarray(0, newline);
			this.#buffer = this.#buffer.subarray(newline + 1);
			if (line.length > this.#maxLineBytes) {
				this.close(
					new RangeError("app-server frame exceeds the configured limit"),
				);
				return;
			}
			if (line.length > 0) {
				const copy = Buffer.from(line);
				void this.#handle(copy).catch((error) => this.close(asError(error)));
			}
			newline = this.#buffer.indexOf(0x0a);
		}
	}

	async #handle(line: Buffer): Promise<void> {
		const value: unknown = JSON.parse(line.toString("utf8"));
		if (!isObject(value))
			throw new TypeError("app-server frame must be an object");
		if ("method" in value) {
			const method = value.method;
			validateMethod(method);
			if ("id" in value) {
				await this.#handleServerRequest({
					id: rpcId(value.id),
					method,
					...(value.params === undefined ? {} : { params: value.params }),
				});
			} else {
				const notification = {
					method,
					...(value.params === undefined ? {} : { params: value.params }),
				};
				this.#notificationQueue = this.#notificationQueue.then(async () =>
					this.#onNotification?.(notification),
				);
				await this.#notificationQueue;
			}
			return;
		}
		if (!("id" in value))
			throw new TypeError("app-server response is missing id");
		if (typeof value.id !== "number" || !Number.isSafeInteger(value.id))
			throw new TypeError("app-server response id is invalid");
		const pending = this.#pending.get(value.id);
		if (!pending) return;
		this.#pending.delete(value.id);
		pending.cleanup();
		if ("error" in value) {
			const error = parseError(value.error);
			pending.reject(
				Object.assign(new Error(error.message), {
					code: error.code,
					data: error.data,
				}),
			);
			return;
		}
		if (!("result" in value)) {
			pending.reject(
				new TypeError("app-server response has neither result nor error"),
			);
			return;
		}
		pending.resolve(value.result);
	}

	async #handleServerRequest(request: RpcServerRequest): Promise<void> {
		if (!this.#onServerRequest) {
			await this.#write({
				id: request.id,
				error: {
					code: -32601,
					message: `unsupported server request: ${request.method}`,
				},
			});
			return;
		}
		try {
			const result = await this.#onServerRequest(request);
			await this.#write({ id: request.id, result: result ?? null });
		} catch (error) {
			await this.#write({
				id: request.id,
				error: { code: -32000, message: publicError(error) },
			});
		}
	}

	async #write(value: unknown): Promise<void> {
		this.#assertOpen();
		const line = `${JSON.stringify(value)}\n`;
		if (Buffer.byteLength(line) > this.#maxLineBytes)
			throw new RangeError(
				"outgoing app-server frame exceeds the configured limit",
			);
		if (!this.#input.write(line, "utf8")) await once(this.#input, "drain");
	}

	#assertOpen(): void {
		if (this.#closedError) throw this.#closedError;
	}
}

function parseError(value: unknown): RpcError {
	if (
		!isObject(value) ||
		typeof value.code !== "number" ||
		typeof value.message !== "string"
	) {
		return { code: -32000, message: "Malformed app-server error" };
	}
	return {
		code: value.code,
		message: value.message.slice(0, 4096),
		...(value.data === undefined ? {} : { data: value.data }),
	};
}

function rpcId(value: unknown): RpcId {
	if (
		(typeof value !== "number" || !Number.isSafeInteger(value)) &&
		(typeof value !== "string" || value.length > 256)
	) {
		throw new TypeError("app-server request id is invalid");
	}
	return value;
}

function validateMethod(value: unknown): asserts value is string {
	if (
		typeof value !== "string" ||
		!/^[A-Za-z][A-Za-z0-9._/-]{0,255}$/.test(value)
	)
		throw new TypeError("RPC method is invalid");
}

function publicError(value: unknown): string {
	const message =
		value instanceof Error ? value.message : "server request failed";
	return [...message]
		.map((character) => (character.charCodeAt(0) < 32 ? " " : character))
		.join("")
		.slice(0, 1024);
}

function asError(value: unknown): Error {
	return value instanceof Error ? value : new Error(String(value));
}

function isObject(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null && !Array.isArray(value);
}
