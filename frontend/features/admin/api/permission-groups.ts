import type {
  CreatePermissionGroupRequest as ContractCreatePermissionGroupRequest,
  DeletePermissionGroupResponseDoc,
  GroupModelsResponseDoc,
  GroupUsersResponseDoc,
  ModelPermissionGroupsResponseDoc,
  PermissionGroupDataResponseDoc,
  PermissionGroupListResponseDoc,
  PermissionGroupModelRuleResponse,
  PermissionGroupResponse,
  UpdatePermissionGroupRequest as ContractUpdatePermissionGroupRequest,
} from "@deeix/api-contract";
import { authedRequest } from "@/shared/api/authed-client";
import { pathParam } from "@/shared/api/http-client";

export type PermissionGroup = PermissionGroupResponse;

export type PermissionGroupModelRuleType = "all" | "vendor" | "protocol" | "upstream";

export type PermissionGroupModelRule = {
  type: PermissionGroupModelRuleType;
  value: string;
};

export type CreatePermissionGroupRequest = ContractCreatePermissionGroupRequest;

export type UpdatePermissionGroupRequest = ContractUpdatePermissionGroupRequest;

type PermissionGroupListData = PermissionGroupListResponseDoc;

type PermissionGroupData = PermissionGroupDataResponseDoc;

type GroupModelsData = GroupModelsResponseDoc;

type GroupUsersData = GroupUsersResponseDoc;

type ModelPermissionGroupsData = ModelPermissionGroupsResponseDoc;

export type DeletePermissionGroupResult = DeletePermissionGroupResponseDoc;

function toPermissionGroupModelRule(
  rule: PermissionGroupModelRuleResponse,
): PermissionGroupModelRule | null {
  switch (rule.ruleType) {
    case "all":
    case "vendor":
    case "protocol":
    case "upstream":
      return { type: rule.ruleType, value: rule.value };
    default:
      return null;
  }
}

export async function listPermissionGroups(accessToken: string): Promise<PermissionGroup[]> {
  const data = await authedRequest<PermissionGroupListData>(
    "/api/v1/admin/permission-groups",
    { accessToken },
    true,
  );
  return data.results ?? [];
}

export async function createPermissionGroup(
  accessToken: string,
  req: CreatePermissionGroupRequest,
): Promise<PermissionGroup> {
  const data = await authedRequest<PermissionGroupData>(
    "/api/v1/admin/permission-groups",
    { method: "POST", accessToken, body: req },
    true,
  );
  return data.group;
}

export async function updatePermissionGroup(
  accessToken: string,
  id: number,
  req: UpdatePermissionGroupRequest,
): Promise<PermissionGroup> {
  const data = await authedRequest<PermissionGroupData>(
    `/api/v1/admin/permission-groups/${pathParam(id)}`,
    { method: "PATCH", accessToken, body: req },
    true,
  );
  return data.group;
}

export async function deletePermissionGroup(accessToken: string, id: number): Promise<DeletePermissionGroupResult> {
  return authedRequest<DeletePermissionGroupResult>(
    `/api/v1/admin/permission-groups/${pathParam(id)}`,
    { method: "DELETE", accessToken },
    true,
  );
}

export async function listGroupModels(
  accessToken: string,
  groupID: number,
): Promise<{ modelIDs: number[]; rules: PermissionGroupModelRule[] }> {
  const data = await authedRequest<GroupModelsData>(
    `/api/v1/admin/permission-groups/${pathParam(groupID)}/models`,
    { accessToken },
    true,
  );
  return {
    modelIDs: data.modelIDs ?? [],
    rules: (data.rules ?? []).flatMap((rule) => {
      const mapped = toPermissionGroupModelRule(rule);
      return mapped ? [mapped] : [];
    }),
  };
}

export async function setGroupModels(
  accessToken: string,
  groupID: number,
  modelIDs: number[],
  rules: PermissionGroupModelRule[] = [],
): Promise<void> {
  await authedRequest<GroupModelsData>(
    `/api/v1/admin/permission-groups/${pathParam(groupID)}/models`,
    { method: "PUT", accessToken, body: { modelIDs, rules: rules.map((rule) => ({ ruleType: rule.type, value: rule.value })) } },
    true,
  );
}

export async function listModelPermissionGroups(
  accessToken: string,
  modelID: number,
): Promise<ModelPermissionGroupsData> {
  const data = await authedRequest<ModelPermissionGroupsData>(
    `/api/v1/admin/models/${pathParam(modelID)}/permission-groups`,
    { accessToken },
    true,
  );
  return {
    manualGroupIDs: data.manualGroupIDs ?? [],
    matchedGroupIDs: data.matchedGroupIDs ?? [],
    effectiveGroupIDs: data.effectiveGroupIDs ?? [],
    unassigned: data.unassigned ?? false,
  };
}

export async function setModelPermissionGroups(
  accessToken: string,
  modelID: number,
  groupIDs: number[],
): Promise<ModelPermissionGroupsData> {
  const data = await authedRequest<ModelPermissionGroupsData>(
    `/api/v1/admin/models/${pathParam(modelID)}/permission-groups`,
    { method: "PUT", accessToken, body: { groupIDs } },
    true,
  );
  return {
    manualGroupIDs: data.manualGroupIDs ?? [],
    matchedGroupIDs: data.matchedGroupIDs ?? [],
    effectiveGroupIDs: data.effectiveGroupIDs ?? [],
    unassigned: data.unassigned ?? false,
  };
}

export async function listGroupUsers(accessToken: string, groupID: number): Promise<number[]> {
  const data = await authedRequest<GroupUsersData>(
    `/api/v1/admin/permission-groups/${pathParam(groupID)}/users`,
    { accessToken },
    true,
  );
  return data.userIDs ?? [];
}

export async function setGroupUsers(
  accessToken: string,
  groupID: number,
  userIDs: number[],
): Promise<void> {
  await authedRequest<GroupUsersData>(
    `/api/v1/admin/permission-groups/${pathParam(groupID)}/users`,
    { method: "PUT", accessToken, body: { userIDs } },
    true,
  );
}
