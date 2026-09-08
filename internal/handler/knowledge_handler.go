package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"Pilot/internal/ai/retriever"
)

const maxKnowledgeRequestBody = 4 << 20

// KnowledgeHandler 提供知识文档导入入口。检索本身由 Agent 的 knowledge_search 工具触发。
type KnowledgeHandler struct {
	ingestor *retriever.Ingestor
}

func NewKnowledgeHandler(ingestor *retriever.Ingestor) *KnowledgeHandler {
	return &KnowledgeHandler{ingestor: ingestor}
}

type knowledgeUploadRequest struct {
	DocID    string   `json:"doc_id"`
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	Source   string   `json:"source"`
	Category string   `json:"category"`
	Tags     []string `json:"tags,omitempty"`
	Version  int      `json:"version"`
}

type knowledgeUploadResponse struct {
	DocID      string `json:"doc_id"`
	ChunkCount int    `json:"chunk_count"`
}

// Upload 处理 POST /api/v1/knowledge/documents。
func (h *KnowledgeHandler) Upload(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.ingestor == nil {
		WriteError(w, http.StatusServiceUnavailable, "knowledge_unavailable", "knowledge ingestor is unavailable")
		return
	}
	var request knowledgeUploadRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxKnowledgeRequestBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		WriteError(w, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		WriteError(w, http.StatusBadRequest, "invalid_request", "request body must contain one JSON object")
		return
	}

	chunks, err := h.ingestor.Ingest(r.Context(), retriever.KnowledgeDocument{
		DocID: request.DocID, Title: request.Title, Content: request.Content,
		Source: request.Source, Category: request.Category, Tags: request.Tags, Version: request.Version,
	})
	if err != nil {
		if errors.Is(err, retriever.ErrInvalidKnowledgeDocument) {
			WriteError(w, http.StatusBadRequest, "invalid_request", "title and content are required")
			return
		}
		WriteError(w, http.StatusInternalServerError, "knowledge_failed", "knowledge document ingestion failed")
		return
	}
	docID := request.DocID
	if docID == "" && len(chunks) > 0 {
		docID = chunks[0].DocID
	}
	writeJSON(w, http.StatusOK, knowledgeUploadResponse{DocID: docID, ChunkCount: len(chunks)})
}
