"use client";

import * as React from "react";

import { dispatchUserSettingsUpdated } from "@/features/settings/events/user-settings-events";
import { useAuthSession } from "@/shared/auth/auth-session-context";
import {
  createSub2KeyBinding,
  deleteSub2KeyBinding,
  listSub2KeyBindings,
  listSub2RemoteKeys,
  type Sub2KeyBindingDTO,
  type Sub2RemoteKeyDTO,
} from "@/shared/api/sub2-key";
import { getUserSettings, patchUserSettings } from "@/shared/api/user-settings";
import {
  isUsableChatKeyBinding,
  resolveDefaultChatKeyBinding,
} from "@/features/chat/hooks/chat-key-binding-validity";
import { createIdempotencyKey } from "@/shared/lib/idempotency-key";

const DEFAULT_KEY_SETTING = "chat.default_sub2_key_binding_id";

export function useChatKeyBindings() {
  const { accessToken } = useAuthSession();
  const [remoteKeys, setRemoteKeys] = React.useState<Sub2RemoteKeyDTO[]>([]);
  const [bindings, setBindings] = React.useState<Sub2KeyBindingDTO[]>([]);
  const [selectedKeyBindingID, setSelectedKeyBindingID] = React.useState("");
  const [loading, setLoading] = React.useState(true);
  const [error, setError] = React.useState("");
  const requestRef = React.useRef(0);
  const selectedRef = React.useRef("");

  const applySelection = React.useCallback((publicID: string) => {
    selectedRef.current = publicID;
    setSelectedKeyBindingID(publicID);
  }, []);

  const persistSelection = React.useCallback(async (publicID: string) => {
    if (!accessToken) return;
    const settings = await patchUserSettings(accessToken, { [DEFAULT_KEY_SETTING]: publicID });
    const saved = settings[DEFAULT_KEY_SETTING]?.trim() ?? "";
    applySelection(saved);
    dispatchUserSettingsUpdated(settings);
  }, [accessToken, applySelection]);

  const refresh = React.useCallback(async () => {
    const requestID = requestRef.current + 1;
    requestRef.current = requestID;
    if (!accessToken) {
      setRemoteKeys([]);
      setBindings([]);
      applySelection("");
      setLoading(false);
      return;
    }
    setLoading(true);
    setError("");
    try {
      const [nextRemoteKeys, nextBindings, settings] = await Promise.all([
        listSub2RemoteKeys(accessToken),
        listSub2KeyBindings(accessToken),
        getUserSettings(accessToken),
      ]);
      if (requestID !== requestRef.current) return;
      setRemoteKeys(nextRemoteKeys);
      setBindings(nextBindings);
      const configured = settings[DEFAULT_KEY_SETTING]?.trim() ?? "";
      applySelection(resolveDefaultChatKeyBinding(nextBindings, configured));
    } catch (caught) {
      if (requestID === requestRef.current) setError(caught instanceof Error ? caught.message : "load failed");
    } finally {
      if (requestID === requestRef.current) setLoading(false);
    }
  }, [accessToken, applySelection]);

  React.useEffect(() => { void refresh(); }, [refresh]);

  const select = React.useCallback(async (remoteKeyID: number) => {
    if (!accessToken) return;
    const requestID = requestRef.current + 1;
    requestRef.current = requestID;
    setLoading(true);
    setError("");
    const existing = bindings.find((item) => item.remoteKeyID === remoteKeyID && isUsableChatKeyBinding(item));
    try {
      const binding = existing ?? await createSub2KeyBinding(accessToken, remoteKeyID, createIdempotencyKey());
      if (requestID !== requestRef.current) return;
      setBindings((current) => [...current.filter((item) => item.publicID !== binding.publicID), binding]);
      await persistSelection(binding.publicID);
    } catch (caught) {
      if (requestID === requestRef.current) setError(caught instanceof Error ? caught.message : "selection failed");
    } finally {
      if (requestID === requestRef.current) setLoading(false);
    }
  }, [accessToken, bindings, persistSelection]);

  const remove = React.useCallback(async (publicID: string) => {
    if (!accessToken) return;
    const requestID = requestRef.current + 1;
    requestRef.current = requestID;
    setLoading(true);
    setError("");
    try {
      await deleteSub2KeyBinding(accessToken, publicID);
      if (requestID === requestRef.current) {
        if (selectedRef.current === publicID) {
          const remaining = bindings.filter((item) => item.publicID !== publicID);
          await persistSelection(resolveDefaultChatKeyBinding(remaining, ""));
        }
        await refresh();
      }
    } catch (caught) {
      if (requestID === requestRef.current) setError(caught instanceof Error ? caught.message : "deletion failed");
    } finally {
      if (requestID === requestRef.current) setLoading(false);
    }
  }, [accessToken, bindings, persistSelection, refresh]);

  return { remoteKeys, bindings, selectedKeyBindingID, loading, error, refresh, select, remove };
}
