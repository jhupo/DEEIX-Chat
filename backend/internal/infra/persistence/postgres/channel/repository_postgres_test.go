package channel

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/testutil"
	"gorm.io/gorm"
)

func TestListModelsPostgresFiltersByActiveUpstream(t *testing.T) {
	db := openChannelPostgresTestDB(t)
	ctx := context.Background()

	upstreams := []model.LLMUpstream{
		{Name: "upstream-a", Status: "active"},
		{Name: "upstream-b", Status: "active"},
		{Name: "inactive-upstream", Status: "inactive"},
	}
	if err := db.Create(&upstreams).Error; err != nil {
		t.Fatalf("create upstreams: %v", err)
	}

	upstreamModels := []model.LLMUpstreamModel{
		{UpstreamID: upstreams[0].ID, BindingCode: "a", UpstreamModelName: "a", Status: "active"},
		{UpstreamID: upstreams[1].ID, BindingCode: "b", UpstreamModelName: "b", Status: "active"},
		{UpstreamID: upstreams[0].ID, BindingCode: "inactive-model", UpstreamModelName: "inactive-model", Status: "inactive"},
		{UpstreamID: upstreams[2].ID, BindingCode: "inactive-upstream-model", UpstreamModelName: "inactive-upstream-model", Status: "active"},
	}
	if err := db.Create(&upstreamModels).Error; err != nil {
		t.Fatalf("create upstream models: %v", err)
	}

	platformModels := []model.LLMPlatformModel{
		{Name: "a-only", Vendor: "openai", Status: "active", SortOrder: 100},
		{Name: "b-only", Vendor: "openai", Status: "active", SortOrder: 200},
		{Name: "shared", Vendor: "openai", Status: "active", SortOrder: 300},
		{Name: "inactive-route", Vendor: "openai", Status: "active", SortOrder: 400},
		{Name: "inactive-upstream-model", Vendor: "openai", Status: "active", SortOrder: 500},
		{Name: "inactive-upstream", Vendor: "openai", Status: "active", SortOrder: 600},
	}
	if err := db.Create(&platformModels).Error; err != nil {
		t.Fatalf("create platform models: %v", err)
	}

	routes := []model.LLMPlatformModelRoute{
		{PlatformModelID: platformModels[0].ID, UpstreamModelID: upstreamModels[0].ID, Protocol: "openai_responses", Status: "active"},
		{PlatformModelID: platformModels[1].ID, UpstreamModelID: upstreamModels[1].ID, Protocol: "openai_responses", Status: "active"},
		{PlatformModelID: platformModels[2].ID, UpstreamModelID: upstreamModels[0].ID, Protocol: "openai_responses", Status: "active"},
		{PlatformModelID: platformModels[2].ID, UpstreamModelID: upstreamModels[1].ID, Protocol: "openai_responses", Status: "active"},
		{PlatformModelID: platformModels[3].ID, UpstreamModelID: upstreamModels[0].ID, Protocol: "openai_responses", Status: "inactive"},
		{PlatformModelID: platformModels[4].ID, UpstreamModelID: upstreamModels[2].ID, Protocol: "openai_responses", Status: "active"},
		{PlatformModelID: platformModels[5].ID, UpstreamModelID: upstreamModels[3].ID, Protocol: "openai_responses", Status: "active"},
	}
	if err := db.Create(&routes).Error; err != nil {
		t.Fatalf("create routes: %v", err)
	}

	items, total, err := NewRepo(db).ListModels(ctx, repository.ListChannelModelsInput{
		Limit:      10,
		UpstreamID: upstreams[0].ID,
		Sort:       "platformModelName_asc",
	})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	got := modelNames(items)
	want := []string{"a-only", "shared"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected model names %v, got %v", want, got)
	}
	assertUpstreamNamesJSON(t, items[1].UpstreamNamesJSON, []string{"upstream-a", "upstream-b"})
}

func TestReorderModelsPostgresUpdatesSubmittedModelsOnly(t *testing.T) {
	db := openChannelPostgresTestDB(t)
	ctx := context.Background()
	upstreamModel := createActiveRouteTarget(t, db)

	models := []model.LLMPlatformModel{
		{Name: "disabled-claude", Vendor: "anthropic", Status: "inactive", SortOrder: 100},
		{Name: "gpt-5.5", Vendor: "openai", Status: "active", SortOrder: 200},
		{Name: "gemini-3.1-pro", Vendor: "google", Status: "active", SortOrder: 300},
		{Name: "claude-fable-5", Vendor: "anthropic", Status: "active", SortOrder: 1000},
	}
	if err := db.Create(&models).Error; err != nil {
		t.Fatalf("create platform models: %v", err)
	}
	createActiveRoutes(t, db, upstreamModel.ID, models[1], models[2], models[3])

	repo := NewRepo(db)
	if err := repo.ReorderModels(ctx, []uint{models[1].ID, models[3].ID, models[2].ID}); err != nil {
		t.Fatalf("ReorderModels() error = %v", err)
	}
	items, _, err := repo.ListModels(ctx, repository.ListChannelModelsInput{
		Limit: 10,
		Sort:  "sortOrder_asc",
	})
	if err != nil {
		t.Fatalf("ListModels() error = %v", err)
	}
	got := modelNames(items)
	want := []string{
		"gpt-5.5",
		"claude-fable-5",
		"gemini-3.1-pro",
		"disabled-claude",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected model order %v, got %v", want, got)
	}
	var disabled model.LLMPlatformModel
	if err := db.First(&disabled, models[0].ID).Error; err != nil {
		t.Fatalf("load disabled model: %v", err)
	}
	if disabled.SortOrder != 100 {
		t.Fatalf("expected disabled model sort order to remain 100, got %d", disabled.SortOrder)
	}
}

func TestListActiveRoutesByModelIncludesPlatformCircuitDefaults(t *testing.T) {
	db := openChannelPostgresTestDB(t)
	ctx := context.Background()
	upstreamModel := createActiveRouteTarget(t, db)

	platformModel := model.LLMPlatformModel{
		Name:               "gpt-circuit",
		Vendor:             "openai",
		Status:             "active",
		CbPolicyMode:       "enforced",
		CbFailureThreshold: 7,
		CbDurationMin:      8,
		CbWindowMin:        9,
	}
	if err := db.Create(&platformModel).Error; err != nil {
		t.Fatalf("create platform model: %v", err)
	}
	if err := db.Create(&model.LLMPlatformModelRoute{
		PlatformModelID:    platformModel.ID,
		UpstreamModelID:    upstreamModel.ID,
		Protocol:           "openai_responses",
		Status:             "active",
		CbFailureThreshold: 2,
		CbDurationMin:      3,
		CbWindowMin:        4,
	}).Error; err != nil {
		t.Fatalf("create route: %v", err)
	}

	rows, err := NewRepo(db).ListActiveRoutesByModel(ctx, platformModel.Name)
	if err != nil {
		t.Fatalf("ListActiveRoutesByModel() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 route, got %d", len(rows))
	}
	row := rows[0]
	if row.PlatformModelCbFailureThreshold != 7 || row.PlatformModelCbDurationMin != 8 || row.PlatformModelCbWindowMin != 9 {
		t.Fatalf("expected platform circuit defaults 7/8/9, got %d/%d/%d",
			row.PlatformModelCbFailureThreshold,
			row.PlatformModelCbDurationMin,
			row.PlatformModelCbWindowMin,
		)
	}
	if row.PlatformModelCbPolicyMode != "enforced" {
		t.Fatalf("expected platform circuit policy enforced, got %q", row.PlatformModelCbPolicyMode)
	}
	if row.ModelCbFailureThreshold != 2 || row.ModelCbDurationMin != 3 || row.ModelCbWindowMin != 4 {
		t.Fatalf("expected route circuit overrides 2/3/4, got %d/%d/%d",
			row.ModelCbFailureThreshold,
			row.ModelCbDurationMin,
			row.ModelCbWindowMin,
		)
	}
}

func openChannelPostgresTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := testutil.Postgres(t)
	if err := db.AutoMigrate(
		&model.LLMUpstream{},
		&model.LLMUpstreamModel{},
		&model.LLMModelVendor{},
		&model.LLMModelDisplayGroup{},
		&model.LLMPlatformModel{},
		&model.LLMPlatformModelRoute{},
		&model.PermissionGroup{},
		&model.PermissionGroupModelAccess{},
		&model.PermissionGroupModelRule{},
		&model.PermissionGroupUserAccess{},
		&model.User{},
	); err != nil {
		t.Fatalf("migrate channel tables: %v", err)
	}
	return db
}

func modelNames(items []ModelListRow) []string {
	results := make([]string, 0, len(items))
	for _, item := range items {
		results = append(results, item.PlatformModelName)
	}
	return results
}

func createActiveRouteTarget(t *testing.T, db *gorm.DB) model.LLMUpstreamModel {
	t.Helper()

	upstream := model.LLMUpstream{Name: "active-upstream", Status: "active"}
	if err := db.Create(&upstream).Error; err != nil {
		t.Fatalf("create active upstream: %v", err)
	}
	upstreamModel := model.LLMUpstreamModel{
		UpstreamID:        upstream.ID,
		BindingCode:       "active-route-target",
		UpstreamModelName: "active-route-target",
		Status:            "active",
	}
	if err := db.Create(&upstreamModel).Error; err != nil {
		t.Fatalf("create active upstream model: %v", err)
	}
	return upstreamModel
}

func createActiveRoutes(t *testing.T, db *gorm.DB, upstreamModelID uint, models ...model.LLMPlatformModel) {
	t.Helper()

	routes := make([]model.LLMPlatformModelRoute, 0, len(models))
	for _, item := range models {
		routes = append(routes, model.LLMPlatformModelRoute{
			PlatformModelID: item.ID,
			UpstreamModelID: upstreamModelID,
			Protocol:        "openai_responses",
			Status:          "active",
		})
	}
	if err := db.Create(&routes).Error; err != nil {
		t.Fatalf("create active routes: %v", err)
	}
}

func assertProtocolsJSON(t *testing.T, raw string, expected []string) {
	t.Helper()

	var actual []string
	if err := json.Unmarshal([]byte(raw), &actual); err != nil {
		t.Fatalf("unmarshal protocols JSON %q: %v", raw, err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("expected protocols %v, got %v", expected, actual)
	}
}

func assertUpstreamNamesJSON(t *testing.T, raw string, expected []string) {
	t.Helper()

	var actual []string
	if err := json.Unmarshal([]byte(raw), &actual); err != nil {
		t.Fatalf("unmarshal upstream names JSON %q: %v", raw, err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("expected upstream names %v, got %v", expected, actual)
	}
}

func containsUint(items []uint, target uint) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
