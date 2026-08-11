"use client";

import * as React from "react";
import { Save } from "lucide-react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { SpinnerLabel } from "@/components/ui/spinner";
import { SettingsFieldEditor } from "@/features/admin/components/sections/shared/settings-runtime-panel";
import { listAdminSettings, patchAdminSettings } from "@/features/admin/api";
import {
  applyLoginDefaults,
  buildLoginSettingsGroups,
  fieldID,
  flattenLoginSettings,
  isRateLimitChildField,
  toEditorField,
  type LoginSettingsField,
} from "@/features/admin/model/login-settings";
import { resolveAdminErrorMessage } from "@/features/admin/utils/admin-error";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";
import { SettingsFieldInset, SettingsFieldItem, SettingsFieldList, SettingsPage, SettingsSection, SettingsSectionSeparator } from "@/shared/components/settings-layout";
import type { PatchSettingItem } from "@/shared/api/settings.types";

export function AdminLoginSettingsPage() {
  const t = useTranslations("adminLogin");
  const loginSettingsGroups = React.useMemo(() => buildLoginSettingsGroups(t), [t]);
  const [settingsMap, setSettingsMap] = React.useState<Record<string, string>>(() => applyLoginDefaults({}));
  const [savedMap, setSavedMap] = React.useState<Record<string, string>>(() => applyLoginDefaults({}));
  const [loading, setLoading] = React.useState(true);
  const [saving, setSaving] = React.useState(false);

  React.useEffect(() => {
    let cancelled = false;
    void (async () => {
      setLoading(true);
      try {
        const token = await resolveAccessToken();
        if (!token) {
          toast.error(t("toast.sessionExpired"), { description: t("toast.signInAgain") });
          return;
        }
        const flattened = flattenLoginSettings(await listAdminSettings(token));
        if (!cancelled) {
          setSettingsMap(flattened);
          setSavedMap(flattened);
        }
      } catch (error) {
        toast.error(t("toast.loadFailed"), { description: resolveAdminErrorMessage(error) });
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [t]);

  const saveGroup = React.useCallback(async (fields: LoginSettingsField[]) => {
    const next = applyLoginDefaults(settingsMap);
    const nextPath = next["auth.login_default_next_path"];
    if (!nextPath.startsWith("/") || nextPath.startsWith("//")) {
      toast.error(t("toast.saveFailed"), { description: t("validation.defaultNextPath") });
      return;
    }
    const items: PatchSettingItem[] = fields
      .map((field) => ({ namespace: field.namespace, key: field.key, value: next[fieldID(field)] ?? "" }))
      .filter((item) => item.value !== (savedMap[`${item.namespace}.${item.key}`] ?? ""));
    if (items.length === 0) return;
    setSaving(true);
    try {
      const token = await resolveAccessToken();
      if (!token) {
        toast.error(t("toast.sessionExpired"), { description: t("toast.signInAgain") });
        return;
      }
      const flattened = flattenLoginSettings(await patchAdminSettings(token, { items }));
      setSettingsMap(flattened);
      setSavedMap(flattened);
      toast.success(t("toast.settingsUpdated"));
    } catch (error) {
      toast.error(t("toast.saveFailed"), { description: resolveAdminErrorMessage(error) });
    } finally {
      setSaving(false);
    }
  }, [savedMap, settingsMap, t]);

  return (
    <SettingsPage>
      {loginSettingsGroups.map((group, index) => {
        const dirty = group.fields.some((field) => (settingsMap[fieldID(field)] ?? "") !== (savedMap[fieldID(field)] ?? ""));
        return (
          <React.Fragment key={group.title}>
            <SettingsSection title={group.title} actions={
              <Button type="button" size="sm" disabled={loading || saving || !dirty} onClick={() => void saveGroup(group.fields)}>
                {saving ? <SpinnerLabel>{t("actions.saving")}</SpinnerLabel> : <><Save className="size-3.5" />{t("actions.save")}</>}
              </Button>
            }>
              <p className="text-sm text-muted-foreground">{group.description}</p>
              <SettingsFieldList>
                {group.fields.map((field) => {
                  const id = fieldID(field);
                  const isRateLimitChild = isRateLimitChildField(field);
                  return (
                    <SettingsFieldItem key={id} className={isRateLimitChild ? "pl-4" : undefined}>
                      <SettingsFieldInset className={isRateLimitChild ? "border-l" : undefined}>
                        <SettingsFieldEditor
                          field={toEditorField(field)}
                          value={settingsMap[id] ?? ""}
                          dirty={(settingsMap[id] ?? "") !== (savedMap[id] ?? "")}
                          disabled={loading || saving || (isRateLimitChild && settingsMap["auth.rate_limit_enabled"] !== "true")}
                          onChange={(value) => setSettingsMap((current) => ({ ...current, [id]: value }))}
                        />
                      </SettingsFieldInset>
                    </SettingsFieldItem>
                  );
                })}
              </SettingsFieldList>
            </SettingsSection>
            {index < loginSettingsGroups.length - 1 ? <SettingsSectionSeparator /> : null}
          </React.Fragment>
        );
      })}
    </SettingsPage>
  );
}
