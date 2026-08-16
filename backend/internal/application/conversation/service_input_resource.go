package conversation

import (
	"context"
	"strings"
)

func (s *Service) ListInputResources(ctx context.Context, userID uint, deviceID, workspaceID, query string) (*GatewayInputResourceCatalog, error) {
	if s.gatewayExecutor == nil || userID == 0 || strings.TrimSpace(deviceID) == "" || strings.TrimSpace(workspaceID) == "" {
		return nil, ErrInvalidExecutionTarget
	}
	catalog, err := s.gatewayExecutor.ListInputResources(ctx, userID, strings.TrimSpace(deviceID), strings.TrimSpace(workspaceID))
	if err != nil {
		return nil, err
	}
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]GatewayInputResource, 0, min(len(catalog.Items), 200))
	counts := map[string]int{"skill": 0, "app-mention": 0}
	for _, item := range catalog.Items {
		if query != "" && !strings.Contains(strings.ToLower(item.Name+" "+item.Description), query) {
			continue
		}
		if counts[item.Kind] == 100 {
			continue
		}
		result = append(result, item)
		counts[item.Kind]++
		if counts["skill"] == 100 && counts["app-mention"] == 100 {
			break
		}
	}
	return &GatewayInputResourceCatalog{Items: result, Ready: catalog.Ready}, nil
}

func selectGatewayInputResources(refs []string, available []GatewayInputResource) ([]GatewayInputResource, error) {
	byRef := make(map[string]GatewayInputResource, len(available))
	for _, item := range available {
		byRef[item.ResourceRef] = item
	}
	selected := make([]GatewayInputResource, 0, len(refs))
	seen := make(map[string]struct{}, len(refs))
	for _, rawRef := range refs {
		resourceRef := strings.TrimSpace(rawRef)
		item, ok := byRef[resourceRef]
		if !ok || resourceRef == "" {
			return nil, ErrInvalidExecutionTarget
		}
		if _, duplicate := seen[resourceRef]; duplicate {
			return nil, ErrInvalidExecutionTarget
		}
		seen[resourceRef] = struct{}{}
		selected = append(selected, item)
	}
	return selected, nil
}
