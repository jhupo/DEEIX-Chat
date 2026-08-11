import type { SettingsGrouped } from "@/shared/api/settings.types";

export type LoginFieldType = "int" | "bool" | "string";

export type LoginSettingsField = {
  namespace: "auth";
  key: "login_default_next_path" | "token_ttl_hours" | "refresh_token_ttl_hours" | "rate_limit_enabled" | "rate_limit_rpm" | "public_auth_rate_limit_rpm";
  label: string;
  description: string;
  type: LoginFieldType;
  placeholder?: string;
};

export type LoginSettingsGroup = {
  title: string;
  description: string;
  fields: LoginSettingsField[];
};

type LoginSettingsTranslator = (key: string) => string;

export function buildLoginSettingsGroups(t: LoginSettingsTranslator): LoginSettingsGroup[] {
  return [
    {
      title: t("groups.loginPage.title"),
      description: t("groups.loginPage.description"),
      fields: [
        { namespace: "auth", key: "login_default_next_path", label: t("fields.loginDefaultNextPath.label"), description: t("fields.loginDefaultNextPath.description"), type: "string", placeholder: "/chat" },
      ],
    },
    {
      title: t("groups.loginSecurity.title"),
      description: t("groups.loginSecurity.description"),
      fields: [
        { namespace: "auth", key: "token_ttl_hours", label: t("fields.tokenTTLHours.label"), description: t("fields.tokenTTLHours.description"), type: "int", placeholder: "24" },
        { namespace: "auth", key: "refresh_token_ttl_hours", label: t("fields.refreshTokenTTLHours.label"), description: t("fields.refreshTokenTTLHours.description"), type: "int", placeholder: "720" },
        { namespace: "auth", key: "rate_limit_enabled", label: t("fields.rateLimitEnabled.label"), description: t("fields.rateLimitEnabled.description"), type: "bool" },
        { namespace: "auth", key: "rate_limit_rpm", label: t("fields.rateLimitRPM.label"), description: t("fields.rateLimitRPM.description"), type: "int", placeholder: "60" },
        { namespace: "auth", key: "public_auth_rate_limit_rpm", label: t("fields.publicAuthRateLimitRPM.label"), description: t("fields.publicAuthRateLimitRPM.description"), type: "int", placeholder: "30" },
      ],
    },
  ];
}

export function fieldID(field: LoginSettingsField): string {
  return `${field.namespace}.${field.key}`;
}

export function flattenLoginSettings(grouped: SettingsGrouped): Record<string, string> {
  const result: Record<string, string> = {};
  for (const item of grouped.auth ?? []) result[`auth.${item.key}`] = item.value ?? "";
  return applyLoginDefaults(result);
}

export function applyLoginDefaults(settings: Record<string, string>): Record<string, string> {
  return {
    ...settings,
    "auth.login_default_next_path": settings["auth.login_default_next_path"]?.trim() || "/chat",
    "auth.token_ttl_hours": settings["auth.token_ttl_hours"]?.trim() || "24",
    "auth.refresh_token_ttl_hours": settings["auth.refresh_token_ttl_hours"]?.trim() || "720",
    "auth.rate_limit_enabled": settings["auth.rate_limit_enabled"] || "false",
    "auth.rate_limit_rpm": settings["auth.rate_limit_rpm"]?.trim() || "60",
    "auth.public_auth_rate_limit_rpm": settings["auth.public_auth_rate_limit_rpm"]?.trim() || "30",
  };
}

export function toEditorField(field: LoginSettingsField) {
  return { id: fieldID(field), label: field.label, description: field.description, type: field.type, placeholder: field.placeholder } as const;
}

export function isRateLimitChildField(field: LoginSettingsField) {
  return field.key === "rate_limit_rpm" || field.key === "public_auth_rate_limit_rpm";
}
