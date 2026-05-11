package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"private-workspace/internal/httputil"
)

type Client interface {
	Query(ctx context.Context, prompt string, contextMessages []Message) (string, error)
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type XAIClient struct {
	apiKey string
	client *http.Client
}

func NewXAIClient(apiKey string) *XAIClient {
	return &XAIClient{
		apiKey: strings.TrimSpace(apiKey),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *XAIClient) Query(ctx context.Context, prompt string, contextMessages []Message) (string, error) {
	if c == nil || c.apiKey == "" {
		return "", errors.New("GROK_API_KEY is missing.")
	}
	messages := []Message{{
		Role:    "system",
		Content: "You are an expert personal productivity assistant. You provide concise, structured, and helpful task and project summaries. You follow strict hierarchical organization (Organization -> Project -> Task). When suggesting priorities, you use the Eisenhower matrix (Urgent/Important).",
	}}
	messages = append(messages, contextMessages...)
	messages = append(messages, Message{Role: "user", Content: prompt})
	payload := map[string]any{
		"messages":    messages,
		"model":       "grok-2-latest",
		"temperature": 0,
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.x.ai/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	res, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		errorBody, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("Grok API Error: %d %s", res.StatusCode, string(errorBody))
	}
	var out struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", errors.New("Grok API returned no choices")
	}
	return out.Choices[0].Message.Content, nil
}

type Handler struct {
	client Client
}

func NewHandler(client Client) *Handler {
	return &Handler{client: client}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/ai/task-summarize", h.TaskSummarize)
	r.Post("/api/ai/task-priority", h.TaskPriority)
}

func (h *Handler) TaskSummarize(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Notes string `json:"notes"`
	}
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	if strings.TrimSpace(req.Notes) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Notes are required")
		return
	}
	if h.client == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "GROK_API_KEY is missing.")
		return
	}
	prompt := "Summarize the following task notes into a concise, one-sentence task summary. Avoid flowery language. Just the facts.\n\nNotes:\n" + req.Notes
	summary, err := h.client.Query(r.Context(), prompt, nil)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"summary": summary})
}

func (h *Handler) TaskPriority(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Summary        string `json:"summary"`
		Notes          string `json:"notes"`
		ProjectContext string `json:"project_context"`
	}
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	if strings.TrimSpace(req.Summary) == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Task summary is required")
		return
	}
	if h.client == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "GROK_API_KEY is missing.")
		return
	}
	prompt := fmt.Sprintf(`Based on the following task details and project context, suggest whether this task should be "Urgent" (requires immediate attention) and "Important" (has high long-term impact) according to the Eisenhower matrix.

Respond with exactly a JSON object in the format:
{ "urgent": boolean, "important": boolean, "reasoning": "string" }

Task Summary: %s
Task Notes: %s
Project Context: %s`, req.Summary, defaultText(req.Notes, "None"), defaultText(req.ProjectContext, "None"))
	result, err := h.client.Query(r.Context(), prompt, nil)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var parsed struct {
		Urgent    bool   `json:"urgent"`
		Important bool   `json:"important"`
		Reasoning string `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(stripJSONFence(result)), &parsed); err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, "AI returned an invalid format.")
		return
	}
	httputil.WriteJSON(w, http.StatusOK, parsed)
}

func stripJSONFence(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "```json")
	value = strings.TrimPrefix(value, "```")
	value = strings.TrimSuffix(value, "```")
	return strings.TrimSpace(value)
}

func defaultText(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
