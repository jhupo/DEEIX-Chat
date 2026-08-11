import { authedRequest } from "@/shared/api/authed-client";
import type { AnnouncementDTO } from "@/shared/api/announcements.types";

export async function listAnnouncements(accessToken: string): Promise<AnnouncementDTO[]> {
  return authedRequest<AnnouncementDTO[]>("/api/v1/announcements", { accessToken }, true);
}

export async function closeAnnouncement(accessToken: string, announcementID: number): Promise<void> {
  await authedRequest<void>(
    `/api/v1/announcements/${encodeURIComponent(String(announcementID))}/close`,
    { method: "POST", accessToken },
    true,
  );
}
