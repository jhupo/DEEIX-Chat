import { authedRequest } from "@/shared/api/authed-client";
import type { PublicModelDTO } from "@/shared/api/model.types";

export type Sub2RemoteKeyDTO = {
  remoteKeyID: number;
  label: string;
  maskedKey: string;
  groupID: number | null;
  groupName: string;
  groupPlatform: string;
  status: string;
  quota: number;
  quotaUsed: number;
  expiresAt: string | null;
  bindingPublicID: string | null;
};

export type Sub2KeyBindingDTO = Omit<Sub2RemoteKeyDTO, "bindingPublicID"> & {
  publicID: string;
  version: number;
  lastValidatedAt: string;
};

export function listSub2RemoteKeys(accessToken: string): Promise<Sub2RemoteKeyDTO[]> {
  return authedRequest<Sub2RemoteKeyDTO[]>("/api/v1/me/sub2-keys", { accessToken }, true);
}

export function listSub2KeyBindings(accessToken: string): Promise<Sub2KeyBindingDTO[]> {
  return authedRequest<Sub2KeyBindingDTO[]>("/api/v1/me/sub2-key-bindings", { accessToken }, true);
}

export function createSub2KeyBinding(accessToken: string, remoteKeyID: number, idempotencyKey: string): Promise<Sub2KeyBindingDTO> {
  return authedRequest<Sub2KeyBindingDTO>(
    "/api/v1/me/sub2-key-bindings",
    { method: "POST", accessToken, body: { remoteKeyID }, headers: { "Idempotency-Key": idempotencyKey } },
    true,
  );
}

export function deleteSub2KeyBinding(accessToken: string, publicID: string): Promise<void> {
  return authedRequest<void>(`/api/v1/me/sub2-key-bindings/${encodeURIComponent(publicID)}`, { method: "DELETE", accessToken }, true);
}

export function listChatModels(accessToken: string, keyBindingID: string): Promise<PublicModelDTO[]> {
  const params = new URLSearchParams({ key_binding_id: keyBindingID });
  return authedRequest<PublicModelDTO[]>(`/api/v1/chat/models?${params.toString()}`, { accessToken }, true);
}
