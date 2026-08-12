import { lstat, realpath } from "node:fs/promises";
import { dirname, isAbsolute, relative, resolve } from "node:path";
import { assertOpaqueRef } from "../protocol/agent-command.js";

export class WorkspaceRegistry {
	readonly #roots = new Map<string, string>();

	async register(workspaceId: string, root: string): Promise<string> {
		assertOpaqueRef(workspaceId, "workspaceId");
		if (!isAbsolute(root))
			throw new TypeError("workspace root must be absolute");
		const canonicalRoot = await realpath(root);
		const info = await lstat(canonicalRoot);
		if (!info.isDirectory())
			throw new TypeError("workspace root must be a directory");
		this.#roots.set(workspaceId, canonicalRoot);
		return canonicalRoot;
	}

	root(workspaceId: string): string {
		assertOpaqueRef(workspaceId, "workspaceId");
		const root = this.#roots.get(workspaceId);
		if (!root) throw new Error(`workspace is not registered: ${workspaceId}`);
		return root;
	}

	async resolvePath(
		workspaceId: string,
		requestedPath: string,
	): Promise<string> {
		if (requestedPath.includes("\0") || isAbsolute(requestedPath))
			throw new TypeError("workspace path must be relative");
		const root = this.root(workspaceId);
		const candidate = resolve(root, requestedPath);
		assertContained(root, candidate);

		const existingParent = await nearestExistingPath(candidate, root);
		const canonicalParent = await realpath(existingParent);
		assertContained(root, canonicalParent);
		return resolve(canonicalParent, relative(existingParent, candidate));
	}
}

function assertContained(root: string, candidate: string): void {
	const pathFromRoot = relative(root, candidate);
	if (
		pathFromRoot === ".." ||
		pathFromRoot.startsWith(`..${process.platform === "win32" ? "\\" : "/"}`) ||
		isAbsolute(pathFromRoot)
	) {
		throw new Error("path escapes the registered workspace");
	}
}

async function nearestExistingPath(
	candidate: string,
	root: string,
): Promise<string> {
	let current = candidate;
	while (true) {
		try {
			await lstat(current);
			return current;
		} catch (error) {
			if (!isMissing(error)) throw error;
		}
		if (samePath(current, root)) return root;
		const parent = dirname(current);
		if (samePath(parent, current))
			throw new Error("path escapes the registered workspace");
		current = parent;
	}
}

function samePath(left: string, right: string): boolean {
	return process.platform === "win32"
		? left.toLowerCase() === right.toLowerCase()
		: left === right;
}

function isMissing(error: unknown): error is NodeJS.ErrnoException {
	return (
		error instanceof Error &&
		"code" in error &&
		(error as NodeJS.ErrnoException).code === "ENOENT"
	);
}
