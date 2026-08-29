package conversation

import (
	"context"
	"testing"

	domainconversation "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/domain/conversation"
	model "github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/models"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/infra/persistence/vectorutil"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/repository"
	"github.com/DEEIX-AI/DEEIX-Chat/backend/internal/testutil"
)

func TestSearchMessageChunksFiltersPostgresBranchBeforeTopK(t *testing.T) {
	const embeddingSignature = "test-model@1536"
	db := testutil.Postgres(t)
	if err := db.AutoMigrate(&model.Message{}, &model.MessageChunk{}); err != nil {
		t.Fatalf("migrate conversation vector models: %v", err)
	}
	if err := db.Exec(`ALTER TABLE chat_message_chunks ADD COLUMN IF NOT EXISTS embedding vector`).Error; err != nil {
		t.Fatalf("add message embedding column: %v", err)
	}

	root := model.Message{ConversationID: 20, UserID: 1, PublicID: "msg_pg_vector_root", Role: "user", Status: "success"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatalf("create root message: %v", err)
	}
	active := model.Message{
		ConversationID: 20, UserID: 1, PublicID: "msg_pg_vector_active", ParentMessageID: &root.ID,
		Role: "assistant", Status: "success",
	}
	if err := db.Create(&active).Error; err != nil {
		t.Fatalf("create active message: %v", err)
	}
	sibling := model.Message{
		ConversationID: 20, UserID: 1, PublicID: "msg_pg_vector_sibling", ParentMessageID: &root.ID,
		Role: "assistant", BranchReason: "retry", Status: "success",
	}
	if err := db.Create(&sibling).Error; err != nil {
		t.Fatalf("create sibling message: %v", err)
	}
	leaf := model.Message{
		ConversationID: 20, UserID: 1, PublicID: "msg_pg_vector_leaf", ParentMessageID: &active.ID,
		Role: "user", Status: "pending",
	}
	if err := db.Create(&leaf).Error; err != nil {
		t.Fatalf("create leaf message: %v", err)
	}

	chunks := []model.MessageChunk{
		{ConversationID: 20, MessageID: active.ID, UserID: 1, Role: "assistant", Content: "active branch target", EmbeddingSignature: embeddingSignature},
		{ConversationID: 20, MessageID: sibling.ID, UserID: 1, Role: "assistant", Content: "closer sibling target", EmbeddingSignature: embeddingSignature},
	}
	if err := db.Create(&chunks).Error; err != nil {
		t.Fatalf("create message chunks: %v", err)
	}
	queryEmbedding := make([]float32, 1536)
	queryEmbedding[0] = 1
	activeEmbedding := make([]float32, 1536)
	activeEmbedding[0], activeEmbedding[1] = 0.8, 0.6
	activeVector, err := vectorutil.PostgresLiteral(activeEmbedding)
	if err != nil {
		t.Fatalf("serialize active embedding: %v", err)
	}
	queryVector, err := vectorutil.PostgresLiteral(queryEmbedding)
	if err != nil {
		t.Fatalf("serialize query embedding: %v", err)
	}
	if err := db.Exec(`UPDATE chat_message_chunks SET embedding = ?::vector WHERE id = ?`, activeVector, chunks[0].ID).Error; err != nil {
		t.Fatalf("write active embedding: %v", err)
	}
	if err := db.Exec(`UPDATE chat_message_chunks SET embedding = ?::vector WHERE id = ?`, queryVector, chunks[1].ID).Error; err != nil {
		t.Fatalf("write sibling embedding: %v", err)
	}

	results, err := NewRepo(db).SearchMessageChunks(context.Background(), repository.MessageChunkSearchInput{
		Scope: repository.HistoricalMessageScope{
			ConversationID: 20,
			UserID:         1,
			LeafMessageID:  leaf.ID,
		},
		QueryEmbedding:     queryEmbedding,
		EmbeddingSignature: embeddingSignature,
		TopK:               1,
	})
	if err != nil {
		t.Fatalf("SearchMessageChunks() error = %v", err)
	}
	if len(results) != 1 || results[0].MessageID != active.ID {
		t.Fatalf("expected active branch result despite closer sibling, got %#v", results)
	}
}

func TestReplaceFileChunksSearchesNearestPostgresChunk(t *testing.T) {
	const embeddingSignature = "test-model@1536"
	db := testutil.Postgres(t)
	if err := db.AutoMigrate(&model.FileObject{}, &model.FileChunk{}); err != nil {
		t.Fatalf("migrate file chunks: %v", err)
	}
	if err := db.Exec(`ALTER TABLE file_chunks ADD COLUMN IF NOT EXISTS embedding vector`).Error; err != nil {
		t.Fatalf("add file chunk embedding column: %v", err)
	}

	queryEmbedding := make([]float32, 1536)
	queryEmbedding[0] = 1
	otherEmbedding := make([]float32, 1536)
	otherEmbedding[1] = 1
	chunks := []domainconversation.FileChunk{
		{FileObjID: 10, UserID: 1, ChunkIndex: 0, Content: "alpha search target", TokenCount: 3, EmbeddingSignature: embeddingSignature},
		{FileObjID: 10, UserID: 1, ChunkIndex: 1, Content: "beta unrelated", TokenCount: 2, EmbeddingSignature: embeddingSignature},
	}

	repo := NewRepo(db)
	ctx := context.Background()
	file := model.FileObject{BaseModel: model.BaseModel{ID: 10}, FileID: "file-vector-test", UserID: 1, Status: "active", EmbedStatus: "processing", EmbedSignature: embeddingSignature}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create file object: %v", err)
	}
	published, err := repo.ReplaceFileChunks(ctx, 10, embeddingSignature, chunks, [][]float32{queryEmbedding, otherEmbedding})
	if err != nil {
		t.Fatalf("replace file chunks: %v", err)
	}
	if !published {
		t.Fatal("replace file chunks did not publish")
	}
	results, err := repo.SearchFileChunks(ctx, 1, []uint{10}, queryEmbedding, embeddingSignature, 2)
	if err != nil {
		t.Fatalf("search file chunks: %v", err)
	}
	if len(results) != 2 || results[0].Content != chunks[0].Content {
		t.Fatalf("expected nearest file chunk first, got %#v", results)
	}
}
