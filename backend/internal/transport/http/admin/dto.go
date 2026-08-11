package admin

import (
	appadmin "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/application/admin"
	domainchannel "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/channel"
	"time"
)

type ErrorDoc struct {
	Message string `json:"message"`
}
type PatchUserRequest struct {
	AvatarURL          *string `json:"avatarURL,omitempty"`
	DisplayName        *string `json:"displayName,omitempty"`
	Timezone           *string `json:"timezone,omitempty"`
	Locale             *string `json:"locale,omitempty"`
	ProfilePreferences *string `json:"profilePreferences,omitempty"`
	Reason             string  `json:"reason,omitempty"`
}

func toAppPatchUserInput(req PatchUserRequest) appadmin.PatchUserInput {
	return appadmin.PatchUserInput{AvatarURL: req.AvatarURL, DisplayName: req.DisplayName, Timezone: req.Timezone, Locale: req.Locale, ProfilePreferences: req.ProfilePreferences, Reason: req.Reason}
}

type CreatePermissionGroupRequest struct {
	Name        string `json:"name" binding:"required,max=64"`
	Description string `json:"description,omitempty"`
}
type UpdatePermissionGroupRequest = CreatePermissionGroupRequest
type PermissionGroupResponse struct {
	ID               uint      `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	IsDefault        bool      `json:"isDefault"`
	ModelCount       int64     `json:"modelCount"`
	ManualModelCount int64     `json:"manualModelCount"`
	RuleModelCount   int64     `json:"ruleModelCount"`
	UserCount        int64     `json:"userCount"`
	ManualUserCount  int64     `json:"manualUserCount"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}
type PermissionGroupListResponse struct {
	Results []PermissionGroupResponse `json:"results"`
}
type PermissionGroupDataResponse struct {
	Group PermissionGroupResponse `json:"group"`
}
type PermissionGroupDeleteSummaryResponse struct {
	ModelAccessCount int64 `json:"modelAccessCount"`
	UserAccessCount  int64 `json:"userAccessCount"`
}
type DeletePermissionGroupResponse struct {
	Deleted bool                                 `json:"deleted"`
	Summary PermissionGroupDeleteSummaryResponse `json:"summary"`
}
type PermissionGroupModelRuleRequest struct {
	RuleType string `json:"ruleType"`
	Value    string `json:"value"`
}
type PermissionGroupModelRuleResponse = PermissionGroupModelRuleRequest
type SetGroupModelsRequest struct {
	ModelIDs []uint                            `json:"modelIDs"`
	Rules    []PermissionGroupModelRuleRequest `json:"rules"`
}
type GroupModelsResponse struct {
	ModelIDs []uint                             `json:"modelIDs"`
	Rules    []PermissionGroupModelRuleResponse `json:"rules"`
}
type SetModelPermissionGroupsRequest struct {
	GroupIDs []uint `json:"groupIDs"`
}
type ModelPermissionGroupsResponse struct {
	ManualGroupIDs    []uint `json:"manualGroupIDs"`
	MatchedGroupIDs   []uint `json:"matchedGroupIDs"`
	EffectiveGroupIDs []uint `json:"effectiveGroupIDs"`
	Unassigned        bool   `json:"unassigned"`
}
type SetGroupUsersRequest struct {
	UserIDs []uint `json:"userIDs"`
}
type GroupUsersResponse struct {
	UserIDs []uint `json:"userIDs"`
}

func toPermissionGroupResponse(item domainchannel.PermissionGroup) PermissionGroupResponse {
	return PermissionGroupResponse{ID: item.ID, Name: item.Name, Description: item.Description, IsDefault: item.IsDefault, ModelCount: item.ModelCount, ManualModelCount: item.ManualModelCount, RuleModelCount: item.RuleModelCount, UserCount: item.UserCount, ManualUserCount: item.ManualUserCount, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt}
}
func toPermissionGroupDeleteSummaryResponse(item domainchannel.PermissionGroupDeleteSummary) PermissionGroupDeleteSummaryResponse {
	return PermissionGroupDeleteSummaryResponse{ModelAccessCount: item.ManualModelCount + item.RuleCount, UserAccessCount: item.ManualUserCount}
}
func toPermissionGroupModelRules(items []PermissionGroupModelRuleRequest) []domainchannel.PermissionGroupModelRule {
	result := make([]domainchannel.PermissionGroupModelRule, 0, len(items))
	for _, item := range items {
		result = append(result, domainchannel.PermissionGroupModelRule{RuleType: item.RuleType, Value: item.Value})
	}
	return result
}
func toPermissionGroupModelRuleResponses(items []domainchannel.PermissionGroupModelRule) []PermissionGroupModelRuleResponse {
	result := make([]PermissionGroupModelRuleResponse, 0, len(items))
	for _, item := range items {
		result = append(result, PermissionGroupModelRuleResponse{RuleType: item.RuleType, Value: item.Value})
	}
	return result
}

type PermissionGroupListResponseDoc = PermissionGroupListResponse
type PermissionGroupDataResponseDoc = PermissionGroupDataResponse
type DeletePermissionGroupResponseDoc = DeletePermissionGroupResponse
type GroupModelsResponseDoc = GroupModelsResponse
type ModelPermissionGroupsResponseDoc = ModelPermissionGroupsResponse
type GroupUsersResponseDoc = GroupUsersResponse
