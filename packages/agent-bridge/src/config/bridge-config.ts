import { mkdir, open, readFile, rename, rm } from "node:fs/promises";
import { dirname, join } from "node:path";

export type BridgeConfig = {
	version: 1;
	cloudUrl: string;
	deviceId: string;
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
	const deviceId = Reflect.get(value, "deviceId");
	if (typeof deviceId !== "string" || !/^agd_[a-f0-9]{32}$/.test(deviceId))
		throw new TypeError("bridge deviceId is invalid");
	return { version: 1, cloudUrl, deviceId };
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
