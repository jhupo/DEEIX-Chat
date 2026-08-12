import { platform } from "node:os";
import { DeviceIdentity } from "../identity/device-identity.js";
import { normalizeCloudUrl, type BridgeConfig } from "../config/bridge-config.js";

type Envelope = { errorMsg?: unknown; data?: unknown };

export class BridgeCloudClient {
	readonly #baseUrl: string;
	readonly #fetch: typeof fetch;

	constructor(baseUrl: string, fetchImplementation: typeof fetch = fetch) {
		this.#baseUrl = normalizeCloudUrl(baseUrl);
		this.#fetch = fetchImplementation;
	}

	async pair(
		enrollmentCode: string,
		deviceName: string,
		identity: DeviceIdentity,
	): Promise<BridgeConfig> {
		if (enrollmentCode.trim().length === 0 || enrollmentCode.length > 128)
			throw new TypeError("enrollment code is invalid");
		if (deviceName.trim().length === 0 || [...deviceName.trim()].length > 128)
			throw new TypeError("device name is invalid");
		const data = await this.#post("/api/v1/agent/bridge/enroll", {
			enrollmentCode: enrollmentCode.trim(),
			name: deviceName.trim(),
			platform: platformName(),
			publicKey: identity.publicKeyBase64Url(),
		});
		const deviceId = requiredString(data, "deviceId", /^agd_[a-f0-9]{32}$/);
		return { version: 1, cloudUrl: this.#baseUrl, deviceId };
	}

	async connectionToken(config: BridgeConfig, identity: DeviceIdentity): Promise<string> {
		const challenge = await this.#post("/api/v1/agent/bridge/token-challenges", {
			deviceId: config.deviceId,
		});
		const challengeId = requiredString(challenge, "challengeId", /^agc_[a-f0-9]{32}$/);
		const challengeText = requiredString(challenge, "challenge", /^deeix_challenge_[A-Za-z0-9_-]+$/);
		const result = await this.#post("/api/v1/agent/bridge/tokens", {
			deviceId: config.deviceId,
			challengeId,
			signature: identity.signBase64Url(challengeText),
		});
		return requiredString(result, "connectionToken", /^deeix_connection_[A-Za-z0-9_-]+$/);
	}

	async #post(path: string, body: object): Promise<unknown> {
		const response = await this.#fetch(`${this.#baseUrl}${path}`, {
			method: "POST",
			headers: { "content-type": "application/json" },
			body: JSON.stringify(body),
		});
		let envelope: Envelope;
		try {
			envelope = (await response.json()) as Envelope;
		} catch {
			throw new Error(`Cloud returned an invalid response (${response.status})`);
		}
		if (!response.ok) {
			const message = typeof envelope.errorMsg === "string" ? envelope.errorMsg : "request failed";
			throw new Error(`Cloud request failed (${response.status}): ${message}`);
		}
		return envelope.data;
	}
}

function requiredString(value: unknown, field: string, pattern: RegExp): string {
	if (typeof value !== "object" || value === null || Array.isArray(value))
		throw new Error("Cloud response data is invalid");
	const result = Reflect.get(value, field);
	if (typeof result !== "string" || !pattern.test(result))
		throw new Error(`Cloud response ${field} is invalid`);
	return result;
}

function platformName(): "windows" | "darwin" | "linux" {
	switch (platform()) {
		case "win32":
			return "windows";
		case "darwin":
			return "darwin";
		default:
			return "linux";
	}
}
