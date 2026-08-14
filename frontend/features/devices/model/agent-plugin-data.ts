export type AgentPluginListItem = {
  id: string;
  name: string;
  marketplace: string;
  version: string;
  source: string;
  keywords: string[];
  installed: boolean;
  enabled: boolean;
  featured: boolean;
};

type JsonRecord = Record<string, unknown>;

function asRecord(value: unknown): JsonRecord | null {
  return value !== null && typeof value === "object" && !Array.isArray(value)
    ? value as JsonRecord
    : null;
}

function stringValue(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

export function normalizeAgentPlugins(data: unknown): AgentPluginListItem[] {
  const root = asRecord(data);
  if (!root || !Array.isArray(root.marketplaces)) return [];

  const featuredIDs = new Set(
    Array.isArray(root.featuredPluginIds)
      ? root.featuredPluginIds.map(stringValue).filter(Boolean)
      : [],
  );

  return root.marketplaces.flatMap((marketplaceValue) => {
    const marketplace = asRecord(marketplaceValue);
    if (!marketplace || !Array.isArray(marketplace.plugins)) return [];
    const marketplaceName = stringValue(marketplace.name);

    return marketplace.plugins.flatMap((pluginValue) => {
      const plugin = asRecord(pluginValue);
      if (!plugin) return [];
      const id = stringValue(plugin.id);
      const name = stringValue(plugin.name) || id;
      if (!id || !name) return [];

      const source = asRecord(plugin.source);
      const sourceLabel = source
        ? stringValue(source.path) || stringValue(source.url) || stringValue(source.source)
        : "";
      const keywords = Array.isArray(plugin.keywords)
        ? plugin.keywords.map(stringValue).filter(Boolean)
        : [];

      return [{
        id,
        name,
        marketplace: marketplaceName,
        version: stringValue(plugin.localVersion) || stringValue(plugin.version),
        source: sourceLabel,
        keywords,
        installed: plugin.installed === true,
        enabled: plugin.enabled === true,
        featured: featuredIDs.has(id),
      }];
    });
  }).sort((left, right) => (
    Number(right.installed) - Number(left.installed) ||
    Number(right.featured) - Number(left.featured) ||
    left.name.localeCompare(right.name)
  ));
}
