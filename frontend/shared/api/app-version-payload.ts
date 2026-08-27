export type AppVersionDTO = {
  product: string;
  version: string;
  commit: string;
  buildTime: string;
  buildID: string;
};

export function parseAppVersionPayload(value: unknown): AppVersionDTO {
  if (!value || typeof value !== "object") {
    throw new Error("version response is invalid");
  }
  const version = {
    product: Reflect.get(value, "product"),
    version: Reflect.get(value, "version"),
    commit: Reflect.get(value, "commit"),
    buildTime: Reflect.get(value, "buildTime"),
    buildID: Reflect.get(value, "buildID"),
  };
  if (Object.values(version).some((item) => typeof item !== "string") || !version.buildID.trim()) {
    throw new Error("version response is invalid");
  }
  return version as AppVersionDTO;
}
