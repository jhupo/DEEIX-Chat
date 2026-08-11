import type { ActiveSessionDTO } from "@/shared/api/auth.types";

type Translate = (key: string, values?: Record<string, string | number>) => string;

export function formatDateTime(value: string | null | undefined, locale = "en-US") {
  if (!value) {
    return "-";
  }

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return "-";
  }

  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "long",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
  }).format(date);
}

export function resolveSessionTitle(session: ActiveSessionDTO, t?: Translate) {
  const browserName = session.browserName.trim();
  const osName = session.osName.trim();
  if (browserName && osName) {
    return `${browserName} (${osName})`;
  }
  return session.deviceLabel.trim() || session.deviceName.trim() || t?.("session.unknownDevice") || "Unknown device";
}

export function resolveSessionLocation(session: ActiveSessionDTO, t?: Translate) {
  const parts = [session.cityName.trim(), session.regionName.trim(), session.countryCode.trim()].filter(Boolean);
  return parts.join(", ") || t?.("session.unknownLocation") || "Unknown location";
}

export function resolveSessionIP(session: ActiveSessionDTO, t?: Translate) {
  return session.clientIP.trim() || t?.("session.unknownIP") || "Unknown IP";
}
