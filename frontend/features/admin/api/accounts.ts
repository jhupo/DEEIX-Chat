import { authedRequest } from "@/shared/api/authed-client";
import type {
  AdminUserDTO,
  RevokeAdminUserSessionsData,
} from "@/features/admin/api/admin.types";
import type { PagePayload } from "@/shared/api/common.types";

import { normalizeAdminPagePayload, resolveAdminPage, type AdminListQueryOptions } from "./shared";

export async function listAdminUsers(
  accessToken: string,
  options: AdminListQueryOptions = {},
): Promise<PagePayload<AdminUserDTO>> {
  const { page, pageSize } = resolveAdminPage(options);
  const params = new URLSearchParams({
    page: String(page),
    page_size: String(pageSize),
  });
  if (options.query?.trim()) {
    params.set("q", options.query.trim());
  }
  const data = await authedRequest<PagePayload<AdminUserDTO>>(
    `/api/v1/admin/users?${params.toString()}`,
    { accessToken },
    true,
  );

  return normalizeAdminPagePayload(data);
}

export async function revokeAdminUserSessions(
  accessToken: string,
  userID: number,
): Promise<RevokeAdminUserSessionsData> {
  return authedRequest<RevokeAdminUserSessionsData>(
    `/api/v1/admin/users/${userID}/revoke-sessions`,
    {
      method: "POST",
      accessToken,
    },
    true,
  );
}
