import { type ChildProcessWithoutNullStreams, execFile, spawn } from "node:child_process";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { JsonLineRpcClient, type RpcClientOptions } from "./rpc-client.js";

export type CodexProcess = {
	child: ChildProcessWithoutNullStreams;
	rpc: JsonLineRpcClient;
	close: () => Promise<void>;
};

const execFileAsync = promisify(execFile);
const SUPPORTED_CODEX_VERSIONS = new Set(["0.147.0", "0.147.0-alpha.6.6"]);
export const BUNDLED_CODEX_EXECUTABLE = "@bundled";

export async function assertCodexVersion(
	executable: string,
): Promise<void> {
	const resolvedExecutable = resolveCodexExecutable(executable);
	const { stdout } = await execFileAsync(resolvedExecutable, ["--version"], {
		windowsHide: true,
		timeout: 10_000,
		maxBuffer: 64 * 1024,
	});
	const actual = parseCodexVersion(stdout);
	if (!actual || !SUPPORTED_CODEX_VERSIONS.has(actual))
		throw new Error(
			`Codex version mismatch: expected ${[...SUPPORTED_CODEX_VERSIONS].join(" or ")}, received ${actual ?? "unknown"}`,
		);
}

export function parseCodexVersion(output: string): string | undefined {
	return /^codex-cli\s+(\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)\s*$/.exec(output)?.[1];
}

export function resolveCodexExecutable(executable: string): string {
	if (executable.length === 0 || executable.includes("\0"))
		throw new TypeError("Codex executable is invalid");
	if (executable !== BUNDLED_CODEX_EXECUTABLE) return executable;
	return resolve(
		dirname(fileURLToPath(import.meta.url)),
		"../../../../codex/bin",
		process.platform === "win32" ? "codex.exe" : "codex",
	);
}

export function startCodexAppServer(
	executable: string,
	options: RpcClientOptions = {},
): CodexProcess {
	const child = spawn(resolveCodexExecutable(executable), ["app-server"], {
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
