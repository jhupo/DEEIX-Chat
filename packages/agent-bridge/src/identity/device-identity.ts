import {
	createPrivateKey,
	createPublicKey,
	generateKeyPairSync,
	sign,
	type KeyObject,
} from "node:crypto";
import { mkdir, open, readFile, rename, rm } from "node:fs/promises";
import { dirname, join } from "node:path";

const identityVersion = 1;

type StoredIdentity = {
	version: 1;
	privateKeyPem: string;
};

export class DeviceIdentity {
	readonly #privateKey: KeyObject;

	private constructor(privateKey: KeyObject) {
		this.#privateKey = privateKey;
	}

	static async loadOrCreate(filePath: string): Promise<DeviceIdentity> {
		if (filePath.trim().length === 0 || filePath.includes("\0"))
			throw new TypeError("device identity path is invalid");
		try {
			return DeviceIdentity.fromStored(await readFile(filePath, "utf8"));
		} catch (error) {
			if (!isMissingFile(error)) throw error;
		}

		const { privateKey } = generateKeyPairSync("ed25519");
		const stored: StoredIdentity = {
			version: identityVersion,
			privateKeyPem: privateKey.export({ format: "pem", type: "pkcs8" }).toString(),
		};
		await writePrivateFileAtomic(filePath, `${JSON.stringify(stored)}\n`);
		return new DeviceIdentity(privateKey);
	}

	static fromStored(value: string): DeviceIdentity {
		let parsed: unknown;
		try {
			parsed = JSON.parse(value);
		} catch {
			throw new Error("device identity is not valid JSON");
		}
		if (
			typeof parsed !== "object" ||
			parsed === null ||
			Array.isArray(parsed) ||
			Reflect.get(parsed, "version") !== identityVersion ||
			typeof Reflect.get(parsed, "privateKeyPem") !== "string"
		) {
			throw new Error("device identity has an unsupported format");
		}
		const privateKey = createPrivateKey(Reflect.get(parsed, "privateKeyPem"));
		if (privateKey.asymmetricKeyType !== "ed25519")
			throw new Error("device identity is not Ed25519");
		return new DeviceIdentity(privateKey);
	}

	publicKeyBase64Url(): string {
		const publicJwk = createPublicKey(this.#privateKey).export({ format: "jwk" });
		if (publicJwk.kty !== "OKP" || publicJwk.crv !== "Ed25519" || !publicJwk.x)
			throw new Error("device public key export failed");
		return publicJwk.x;
	}

	signBase64Url(value: string): string {
		if (value.length === 0 || value.length > 256)
			throw new TypeError("device challenge is invalid");
		return sign(null, Buffer.from(value, "utf8"), this.#privateKey).toString("base64url");
	}
}

async function writePrivateFileAtomic(filePath: string, value: string): Promise<void> {
	const directory = dirname(filePath);
	await mkdir(directory, { recursive: true });
	const temporaryPath = join(
		directory,
		`.${process.pid}-${Date.now()}-${Math.random().toString(16).slice(2)}.tmp`,
	);
	let handle;
	try {
		handle = await open(temporaryPath, "wx", 0o600);
		await handle.writeFile(value, "utf8");
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

function isMissingFile(error: unknown): boolean {
	return (
		typeof error === "object" &&
		error !== null &&
		Reflect.get(error, "code") === "ENOENT"
	);
}
