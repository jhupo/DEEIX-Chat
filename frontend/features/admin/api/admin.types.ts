import type {
  AdminUserResponse,
  AuditLogResponse,
  AuthEventResponse,
  ConversationEventResponse,
  RevokeUserSessionsResponse,
  SystemEventResponse,
} from "@deeix/api-contract";
import type { PagePayload } from "@/shared/api/common.types";

export type AdminUserDTO = AdminUserResponse;

export type RevokeAdminUserSessionsData = RevokeUserSessionsResponse;

export type AdminUserAuthEventDTO = AuthEventResponse;

export type AdminAuditLogDTO = AuditLogResponse;

export type AdminSystemEventDTO = SystemEventResponse;

export type AdminConversationEventDTO = ConversationEventResponse;

export type ListAdminUsersResult = PagePayload<AdminUserDTO>;
export type ListAdminUserAuthEventsResult = PagePayload<AdminUserAuthEventDTO>;
export type ListAdminAuditLogsResult = PagePayload<AdminAuditLogDTO>;
export type ListAdminSystemEventsResult = PagePayload<AdminSystemEventDTO>;
export type ListAdminConversationEventsResult = PagePayload<AdminConversationEventDTO>;

export type TikaRuntimeStatus =
  | "running"
  | "stopped"
  | "unhealthy"
  | "failed"
  | "unavailable"
  | "unconfigured"
  | "created"
  | "exited"
  | "paused"
  | "restarting";

export type AdminServiceRuntimeView = {
  source: "external" | "managed";
  baseURL: string;
  containerName: string;
  image: string;
  network: string;
  status: TikaRuntimeStatus | string;
  reachable: boolean;
  message: string;
};

export type AdminTikaRuntimeView = AdminServiceRuntimeView;
export type AdminDoclingRuntimeView = AdminServiceRuntimeView;
export type AdminTesseractRuntimeView = AdminServiceRuntimeView;
export type AdminRapidOCRRuntimeView = AdminServiceRuntimeView;
export type AdminMinerURuntimeView = AdminServiceRuntimeView;
export type AdminEmbeddingRuntimeView = AdminServiceRuntimeView;
