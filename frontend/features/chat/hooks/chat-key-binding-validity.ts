export type ChatKeyBindingValidityInput = {
  status: string;
  expiresAt: string | null;
};

export function isUsableChatKeyBinding(binding: ChatKeyBindingValidityInput, now = Date.now()): boolean {
  if (binding.status.trim().toLowerCase() !== "active") return false;
  if (binding.expiresAt === null) return true;
  const expiresAt = new Date(binding.expiresAt).getTime();
  return Number.isFinite(expiresAt) && expiresAt > now;
}

export function resolveDefaultChatKeyBinding<T extends ChatKeyBindingValidityInput & { publicID: string }>(
  bindings: T[],
  configuredPublicID: string,
  now = Date.now(),
): string {
  const configured = bindings.find(
    (binding) => binding.publicID === configuredPublicID && isUsableChatKeyBinding(binding, now),
  );
  return configured?.publicID ?? bindings.find((binding) => isUsableChatKeyBinding(binding, now))?.publicID ?? "";
}
