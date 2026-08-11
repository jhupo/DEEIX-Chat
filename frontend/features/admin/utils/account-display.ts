import type { AdminUserDTO } from "@/features/admin/api/admin.types";

const USER_STATUS_LABELS: Record<string, string> = {
  pending_activation: "Pending activation",
  active: "Active",
  locked: "Locked",
  suspended: "Suspended",
  deactivated: "Deactivated",
};

const AUTH_EVENT_RESULT_LABELS: Record<string, string> = {
  success: "Success",
  failure: "Failed",
  blocked: "Blocked",
};


export function formatDateTime(value: string | null | undefined, locale = "en-US"): string {
  if (!value) {
    return "-";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }

  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

export function resolveValue(value: string | null | undefined): string {
  return value?.trim() || "-";
}

export function resolveUserStatusLabel(value: string | null | undefined): string {
  const key = value?.trim() ?? "";
  return USER_STATUS_LABELS[key] ?? resolveValue(value);
}

export function resolveAuthEventResultLabel(value: string | null | undefined): string {
  const key = value?.trim() ?? "";
  return AUTH_EVENT_RESULT_LABELS[key] ?? resolveValue(value);
}


export function resolveUserInitial(user: AdminUserDTO): string {
  const source = user.displayName.trim() || user.username.trim() || user.publicID.trim() || String(user.id);
  return source.charAt(0).toUpperCase();
}

export function resolveCreateUserInitial(username: string, displayName: string): string {
  return (displayName.trim() || username.trim() || "U").charAt(0).toUpperCase();
}

export function resolveDetailValue(value: string | number | null | undefined): string {
  if (value === null || value === undefined) {
    return "-";
  }
  if (typeof value === "string") {
    return value.trim() || "-";
  }
  return String(value);
}
