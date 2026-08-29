package memory

import (
	"context"
	"testing"

	domainmemory "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/memory"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/testutil"
)

func TestUserMemoryPostgresVectorLifecycle(t *testing.T) {
	const embeddingSignature = "test-model@1536"
	db := testutil.Postgres(t)
	if err := db.AutoMigrate(&model.UserMemory{}); err != nil {
		t.Fatalf("migrate user memories: %v", err)
	}
	if err := db.Exec(`ALTER TABLE user_memories ADD COLUMN IF NOT EXISTS embedding vector(1536)`).Error; err != nil {
		t.Fatalf("add user memory embedding column: %v", err)
	}

	ctx := context.Background()
	repo := NewRepo(db)
	target := &domainmemory.UserMemory{
		UserID:    1,
		MemoryKey: "favorite_topic",
		Value:     "likes databases",
		Scope:     "global",
		UpdatedBy: "system",
	}
	other := &domainmemory.UserMemory{
		UserID:    1,
		MemoryKey: "favorite_color",
		Value:     "likes green",
		Scope:     "global",
		UpdatedBy: "system",
	}
	if err := repo.UpsertUserMemory(ctx, target); err != nil {
		t.Fatalf("upsert target memory: %v", err)
	}
	if err := repo.UpsertUserMemory(ctx, other); err != nil {
		t.Fatalf("upsert other memory: %v", err)
	}

	queryEmbedding := make([]float32, 1536)
	queryEmbedding[0] = 1
	otherEmbedding := make([]float32, 1536)
	otherEmbedding[1] = 1
	if err := repo.UpsertUserMemoryEmbedding(ctx, 1, target.MemoryKey, target.Value, queryEmbedding, embeddingSignature); err != nil {
		t.Fatalf("embed target memory: %v", err)
	}
	if err := repo.UpsertUserMemoryEmbedding(ctx, 1, other.MemoryKey, other.Value, otherEmbedding, embeddingSignature); err != nil {
		t.Fatalf("embed other memory: %v", err)
	}

	results, err := repo.SearchUserMemoriesByEmbedding(ctx, 1, queryEmbedding, embeddingSignature, 2, 0)
	if err != nil {
		t.Fatalf("search embedded memories: %v", err)
	}
	if len(results) != 2 || results[0].MemoryKey != target.MemoryKey {
		t.Fatalf("expected target memory to be nearest, got %#v", results)
	}

	target.Value = "likes vector databases"
	if err := repo.UpsertUserMemory(ctx, target); err != nil {
		t.Fatalf("update target memory: %v", err)
	}
	results, err = repo.SearchUserMemoriesByEmbedding(ctx, 1, queryEmbedding, embeddingSignature, 2, 0.5)
	if err != nil {
		t.Fatalf("search after target update: %v", err)
	}
	for _, result := range results {
		if result.MemoryKey == target.MemoryKey {
			t.Fatalf("updated memory retained a stale embedding: %#v", results)
		}
	}

	if err := repo.UpsertUserMemoryEmbedding(ctx, 1, target.MemoryKey, target.Value, queryEmbedding, embeddingSignature); err != nil {
		t.Fatalf("re-embed target memory: %v", err)
	}
	results, err = repo.SearchUserMemoriesByEmbedding(ctx, 1, queryEmbedding, embeddingSignature, 2, 0)
	if err != nil {
		t.Fatalf("search re-embedded memories: %v", err)
	}
	if len(results) != 2 || results[0].MemoryKey != target.MemoryKey || results[0].Value != target.Value {
		t.Fatalf("expected re-embedded target memory first, got %#v", results)
	}

	if err := repo.DeleteUserMemory(ctx, 1, target.MemoryKey); err != nil {
		t.Fatalf("delete target memory: %v", err)
	}
	var deletedCount int64
	if err := db.Unscoped().Model(&model.UserMemory{}).
		Where("user_id = ? AND memory_key = ?", 1, target.MemoryKey).
		Count(&deletedCount).Error; err != nil {
		t.Fatalf("count deleted target memory: %v", err)
	}
	if deletedCount != 0 {
		t.Fatalf("expected physical delete of target memory, found %d rows", deletedCount)
	}
	results, err = repo.SearchUserMemoriesByEmbedding(ctx, 1, queryEmbedding, embeddingSignature, 2, 0)
	if err != nil {
		t.Fatalf("search after target delete: %v", err)
	}
	if len(results) != 1 || results[0].MemoryKey != other.MemoryKey {
		t.Fatalf("expected only other memory after delete, got %#v", results)
	}
}
