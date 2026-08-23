import { authedRequest } from "@/shared/api/authed-client";

export type ConversationPluginDTO = {
  key: string;
  label: string;
  description: string;
  enabled: boolean;
  resourceRef: string;
};

export async function listConversationPlugins(
  accessToken: string,
  includeDisabled = false,
): Promise<ConversationPluginDTO[]> {
  const query = includeDisabled ? "?includeDisabled=true" : "";
  return authedRequest<ConversationPluginDTO[]>(`/api/v1/plugins${query}`, { accessToken }, true);
}
