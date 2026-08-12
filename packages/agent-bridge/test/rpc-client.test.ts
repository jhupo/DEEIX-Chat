import assert from "node:assert/strict";
import { PassThrough } from "node:stream";
import test from "node:test";
import { JsonLineRpcClient } from "../src/providers/codex/rpc-client.js";

test("RPC client handles split responses, ordered notifications, and server requests", async () => {
	const input = new PassThrough();
	const output = new PassThrough();
	const sent = lineReader(input);
	const notifications: number[] = [];
	const firstNotificationGate = deferred<void>();
	const rpc = new JsonLineRpcClient(input, output, {
		onNotification: async ({ params }) => {
			const sequence = (params as { sequence: number }).sequence;
			if (sequence === 1) await firstNotificationGate.promise;
			notifications.push(sequence);
		},
		onServerRequest: ({ method }) => ({
			decision:
				method === "item/commandExecution/requestApproval"
					? "accept"
					: "decline",
		}),
	});

	const resultPromise = rpc.request<{ ok: boolean }>("initialize", {
		clientInfo: { name: "deeix-bridge" },
	});
	const request = JSON.parse(await sent.next()) as {
		id: number;
		method: string;
	};
	assert.equal(request.method, "initialize");
	const response = `${JSON.stringify({ id: request.id, result: { ok: true } })}\n`;
	output.write(response.slice(0, 7));
	output.write(response.slice(7));
	assert.deepEqual(await resultPromise, { ok: true });

	output.write(
		`${JSON.stringify({ method: "item/started", params: { sequence: 1 } })}\n`,
	);
	output.write(
		`${JSON.stringify({ method: "item/completed", params: { sequence: 2 } })}\n`,
	);
	output.write(
		`${JSON.stringify({ id: "approval_1", method: "item/commandExecution/requestApproval", params: {} })}\n`,
	);
	const approval = JSON.parse(await sent.next()) as {
		id: string;
		result: { decision: string };
	};
	assert.deepEqual(approval, {
		id: "approval_1",
		result: { decision: "accept" },
	});
	assert.deepEqual(notifications, []);
	firstNotificationGate.resolve();
	await eventually(() => notifications.length === 2);
	assert.deepEqual(notifications, [1, 2]);
	rpc.close();
});

test("RPC client returns method-not-found for an unhandled server request", async () => {
	const input = new PassThrough();
	const output = new PassThrough();
	const sent = lineReader(input);
	const rpc = new JsonLineRpcClient(input, output);
	output.write(
		`${JSON.stringify({ id: 7, method: "unknown/request", params: {} })}\n`,
	);
	const response = JSON.parse(await sent.next()) as {
		id: number;
		error: { code: number };
	};
	assert.equal(response.id, 7);
	assert.equal(response.error.code, -32601);
	rpc.close();
});

function lineReader(stream: PassThrough): { next: () => Promise<string> } {
	const lines: string[] = [];
	const waiters: Array<(value: string) => void> = [];
	let buffer = "";
	stream.setEncoding("utf8");
	stream.on("data", (chunk: string) => {
		buffer += chunk;
		let newline = buffer.indexOf("\n");
		while (newline >= 0) {
			const line = buffer.slice(0, newline);
			buffer = buffer.slice(newline + 1);
			const waiter = waiters.shift();
			if (waiter) waiter(line);
			else lines.push(line);
			newline = buffer.indexOf("\n");
		}
	});
	return {
		next: async () => {
			const line = lines.shift();
			if (line !== undefined) return line;
			return new Promise<string>((resolve) => waiters.push(resolve));
		},
	};
}

function deferred<T>(): { promise: Promise<T>; resolve: (value: T) => void } {
	let resolve!: (value: T) => void;
	const promise = new Promise<T>((done) => {
		resolve = done;
	});
	return { promise, resolve };
}

async function eventually(predicate: () => boolean): Promise<void> {
	for (let index = 0; index < 100; index += 1) {
		if (predicate()) return;
		await new Promise((resolve) => setTimeout(resolve, 5));
	}
	assert.fail("condition was not met");
}
