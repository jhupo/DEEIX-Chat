import { type ChildProcessWithoutNullStreams, execFile, spawn } from "node:child_process";
import { promisify } from "node:util";
import { JsonLineRpcClient, type RpcClientOptions } from "./rpc-client.js";

export type CodexProcess = {
	child: ChildProcessWithoutNullStreams;
	rpc: JsonLineRpcClient;
	close: () => Promise<void>;
};

const execFileAsync = promisify(execFile);

export async function assertCodexVersion(
	executable: string,
	expected = "0.147.0",
): Promise<void> {
	if (executable.length === 0 || executable.includes("\0"))
		throw new TypeError("Codex executable is invalid");
	const { stdout } = await execFileAsync(executable, ["--version"], {
		windowsHide: true,
		timeout: 10_000,
		maxBuffer: 64 * 1024,
	});
	const actual = /^codex-cli\s+(\d+\.\d+\.\d+)\s*$/.exec(stdout)?.[1];
	if (actual !== expected)
		throw new Error(`Codex version mismatch: expected ${expected}, received ${actual ?? "unknown"}`);
}

export function startCodexAppServer(
	executable: string,
	options: RpcClientOptions = {},
): CodexProcess {
	if (executable.length === 0 || executable.includes("\0"))
		throw new TypeError("Codex executable is invalid");
	const child = spawn(executable, ["app-server"], {
		shell: false,
		stdio: ["pipe", "pipe", "pipe"],
		windowsHide: true,
	});
	const rpc = new JsonLineRpcClient(child.stdin, child.stdout, options);
	child.stderr.resume();
	child.once("error", (error) => rpc.close(error));
	child.once("exit", (code, signal) =>
		rpc.close(
			new Error(
				`codex app-server exited (code=${code ?? "none"}, signal=${signal ?? "none"})`,
			),
		),
	);

	return {
		child,
		rpc,
		close: async () => {
			rpc.close();
			if (child.exitCode !== null || child.signalCode !== null) return;
			const exited = new Promise<void>((resolve) =>
				child.once("exit", () => resolve()),
			);
			child.kill();
			await exited;
		},
	};
}
