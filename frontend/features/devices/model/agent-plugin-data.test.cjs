const assert = require("node:assert/strict");
const test = require("node:test");

const { normalizeAgentPlugins } = require("./agent-plugin-data.ts");

test("normalizes app-server plugin marketplaces", () => {
  const plugins = normalizeAgentPlugins({
    featuredPluginIds: ["browser@openai"],
    marketplaces: [{
      name: "openai",
      plugins: [
        { id: "linear@openai", name: "Linear", installed: false, enabled: false },
        {
          id: "browser@openai",
          name: "Browser",
          installed: true,
          enabled: true,
          localVersion: "1.2.3",
          keywords: ["web", "automation"],
          source: { source: "remote" },
        },
      ],
    }],
  });

  assert.deepEqual(plugins.map((item) => item.id), ["browser@openai", "linear@openai"]);
  assert.equal(plugins[0].featured, true);
  assert.equal(plugins[0].version, "1.2.3");
  assert.deepEqual(normalizeAgentPlugins({ marketplaces: "invalid" }), []);
});
