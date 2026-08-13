import { mkdir, open, readFile, rename, rm } from "node:fs/promises";
import { dirname, isAbsolute, join } from "node:path";

export type BridgeConfig = {
	version: 1;
	cloudUrl: string;
	userPublicID: string;
	deviceId: string;
	profileId: string;
	codexExecutable: string;
	workspaces: Array<{ workspaceId: string; root: string; name: string }>;
};

export async function readBridgeConfig(filePath: string): Promise<BridgeConfig> {
	return parseBridgeConfig(JSON.parse(await readFile(filePath, "utf8")));
}

export async function writeBridgeConfig(
	filePath: string,
	config: BridgeConfig,
): Promise<void> {
	const normalized = parseBridgeConfig(config);
	const directory = dirname(filePath);
	await mkdir(directory, { recursive: true });
	const temporaryPath = join(
		directory,
		`.${process.pid}-${Date.now()}-${Math.random().toString(16).slice(2)}.tmp`,
	);
	let handle;
	try {
		handle = await open(temporaryPath, "wx", 0o600);
		await handle.writeFile(`${JSON.stringify(normalized, null, 2)}\n`, "utf8");
		await handle.sync();
		await handle.close();
		handle = undefined;
		await rename(temporaryPath, filePath);
	} catch (error) {
		await handle?.close().catch(() => undefined);
		await rm(temporaryPath, { force: true }).catch(() => undefined);
		throw error;
	}
}

export function parseBridgeConfig(value: unknown): BridgeConfig {
	if (typeof value !== "object" || value === null || Array.isArray(value))
		throw new TypeError("bridge config must be an object");
	if (Reflect.get(value, "version") !== 1)
		throw new TypeError("bridge config version is unsupported");
	const cloudUrl = normalizeCloudUrl(Reflect.get(value, "cloudUrl"));
	const userPublicID = Reflect.get(value, "userPublicID");
	const deviceId = Reflect.get(value, "deviceId");
	const profileId = Reflect.get(value, "profileId");
	const codexExecutable = Reflect.get(value, "codexExecutable");
	const rawWorkspaces = Reflect.get(value, "workspaces");
	if (typeof userPublicID !== "string" || !/^[a-f0-9]{32}$/.test(userPublicID))
		throw new TypeError("bridge userPublicID is invalid");
	if (typeof deviceId !== "string" || !/^agd_[a-f0-9]{32}$/.test(deviceId))
		throw new TypeError("bridge deviceId is invalid");
	if (typeof profileId !== "string" || !/^[A-Za-z0-9._:-]{1,64}$/.test(profileId))
		throw new TypeError("bridge profileId is invalid");
	if (typeof codexExecutable !== "string" || codexExecutable.length === 0 || codexExecutable.length > 2048 || codexExecutable.includes("\0"))
		throw new TypeError("bridge codexExecutable is invalid");
	if (!Array.isArray(rawWorkspaces) || rawWorkspaces.length === 0 || rawWorkspaces.length > 128)
		throw new TypeError("bridge workspaces are invalid");
	const workspaces = rawWorkspaces.map(parseWorkspace);
	if (new Set(workspaces.map(({ workspaceId }) => workspaceId)).size !== workspaces.length)
		throw new TypeError("bridge workspaceId values must be unique");
	return { version: 1, cloudUrl, userPublicID, deviceId, profileId, codexExecutable, workspaces };
}

function parseWorkspace(value: unknown): { workspaceId: string; root: string; name: string } {
	if (typeof value !== "object" || value === null || Array.isArray(value))
		throw new TypeError("bridge workspace must be an object");
	const workspaceId = Reflect.get(value, "workspaceId");
	const root = Reflect.get(value, "root");
	const name = Reflect.get(value, "name");
	if (typeof workspaceId !== "string" || !/^[A-Za-z0-9._:-]{1,64}$/.test(workspaceId))
		throw new TypeError("bridge workspaceId is invalid");
	if (typeof root !== "string" || !isAbsolute(root) || root.includes("\0"))
		throw new TypeError("bridge workspace root is invalid");
	if (typeof name !== "string" || name.trim().length === 0 || [...name.trim()].length > 128)
		throw new TypeError("bridge workspace name is invalid");
	return { workspaceId, root, name: name.trim() };
}

export function normalizeCloudUrl(value: unknown): string {
	if (typeof value !== "string" || value.length > 2048)
		throw new TypeError("cloud URL is invalid");
	let parsed: URL;
	try {
		parsed = new URL(value);
	} catch {
		throw new TypeError("cloud URL is invalid");
	}
	if (!['https:', 'http:'].includes(parsed.protocol) || parsed.username || parsed.password)
		throw new TypeError("cloud URL is invalid");
	if (parsed.search || parsed.hash)
		throw new TypeError("cloud URL must not contain query or fragment");
	parsed.pathname = parsed.pathname.replace(/\/+$/, "");
	return parsed.toString().replace(/\/$/, "");
}
