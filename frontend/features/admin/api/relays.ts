import type {
  ConnectorRequest,
  ConnectorResponse,
  RouteRequest,
  RouteResponse,
} from "@deeix/api-contract";
import { authedRequest } from "@/shared/api/authed-client";
import { pathParam } from "@/shared/api/http-client";

export type AdminRelayConnector = ConnectorResponse;
export type AdminRelayIngressRoute = RouteResponse;
export type RelayConnectorInput = ConnectorRequest;
export type RelayRouteInput = RouteRequest;

export async function listAdminRelayConnectors(accessToken: string) {
  return authedRequest<AdminRelayConnector[]>(
    "/api/v1/admin/relays/connectors",
    { accessToken },
    true,
  );
}

export async function createAdminRelayConnector(accessToken: string, payload: RelayConnectorInput) {
  return authedRequest<AdminRelayConnector>(
    "/api/v1/admin/relays/connectors",
    { method: "POST", accessToken, body: payload },
    true,
  );
}

export async function updateAdminRelayConnector(accessToken: string, id: string, payload: RelayConnectorInput) {
  return authedRequest<AdminRelayConnector>(
    `/api/v1/admin/relays/connectors/${pathParam(id)}`,
    { method: "PATCH", accessToken, body: payload },
    true,
  );
}

export async function deleteAdminRelayConnector(accessToken: string, id: string) {
  return authedRequest<{ deleted: boolean }>(
    `/api/v1/admin/relays/connectors/${pathParam(id)}`,
    { method: "DELETE", accessToken },
    true,
  );
}

export async function listAdminRelayRoutes(accessToken: string) {
  return authedRequest<AdminRelayIngressRoute[]>(
    "/api/v1/admin/relays/routes",
    { accessToken },
    true,
  );
}

export async function createAdminRelayRoute(accessToken: string, payload: RelayRouteInput) {
  return authedRequest<AdminRelayIngressRoute>(
    "/api/v1/admin/relays/routes",
    { method: "POST", accessToken, body: payload },
    true,
  );
}

export async function updateAdminRelayRoute(accessToken: string, id: number, payload: RelayRouteInput) {
  return authedRequest<AdminRelayIngressRoute>(
    `/api/v1/admin/relays/routes/${pathParam(id)}`,
    { method: "PATCH", accessToken, body: payload },
    true,
  );
}

export async function deleteAdminRelayRoute(accessToken: string, id: number) {
  return authedRequest<{ deleted: boolean }>(
    `/api/v1/admin/relays/routes/${pathParam(id)}`,
    { method: "DELETE", accessToken },
    true,
  );
}
