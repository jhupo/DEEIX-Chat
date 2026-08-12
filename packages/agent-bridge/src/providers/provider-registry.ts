import { assertOpaqueRef } from "../protocol/agent-command.js";
import type { ProviderAdapter } from "./provider-adapter.js";

export class ProviderRegistry {
	readonly #adapters = new Map<string, ProviderAdapter>();

	register(profileId: string, adapter: ProviderAdapter): void {
		assertOpaqueRef(profileId, "profileId");
		if (this.#adapters.has(profileId))
			throw new Error(`profile already registered: ${profileId}`);
		this.#adapters.set(profileId, adapter);
	}

	get(profileId: string): ProviderAdapter {
		assertOpaqueRef(profileId, "profileId");
		const adapter = this.#adapters.get(profileId);
		if (!adapter) throw new Error(`profile is not registered: ${profileId}`);
		return adapter;
	}

	async remove(profileId: string): Promise<void> {
		const adapter = this.get(profileId);
		this.#adapters.delete(profileId);
		await adapter.close();
	}

	async close(): Promise<void> {
		const adapters = [...this.#adapters.values()];
		this.#adapters.clear();
		await Promise.allSettled(adapters.map((adapter) => adapter.close()));
	}
}
