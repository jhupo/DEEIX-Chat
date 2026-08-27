import { parseAppVersionPayload, type AppVersionDTO } from "@/shared/api/app-version-payload";
import { resolveApiBaseURL } from "@/shared/api/http-client";

export type { AppVersionDTO } from "@/shared/api/app-version-payload";

export async function getAppVersion(): Promise<AppVersionDTO> {
  const response = await fetch(`${resolveApiBaseURL()}/api/v1/version`, {
    cache: "no-store",
    credentials: "include",
  });
  if (!response.ok) {
    throw new Error(`version request failed: ${response.status}`);
  }
  return parseAppVersionPayload(await response.json());
}
