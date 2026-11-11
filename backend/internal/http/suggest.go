package httpapi

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func suggestHandler() http.HandlerFunc {
	baseURL := cmp.Or(os.Getenv("OLLAMA_URL"), "http://127.0.0.1:11434")
	endpoint := strings.TrimRight(baseURL, "/") + "/api/generate"
	model := cmp.Or(os.Getenv("OLLAMA_MODEL"), "qwen2.5:3b")
	client := &http.Client{Timeout: 30 * time.Second}

	return func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Description string `json:"description"`
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			http.Error(w, "Description is too long.", http.StatusRequestEntityTooLarge)
			return
		}
		if err != nil || !utf8.Valid(body) || json.Unmarshal(body, &input) != nil {
			http.Error(w, "Expected a JSON description.", http.StatusBadRequest)
			return
		}
		description := strings.TrimSpace(input.Description)
		if description == "" || utf8.RuneCountInString(description) > 400 {
			http.Error(w, "Describe a video in 1–400 characters.", http.StatusBadRequest)
			return
		}
		payload, _ := json.Marshal(map[string]any{
			"model": model, "prompt": description, "stream": false,
			"system":  "Turn the viewer's description into one short YouTube search query. Reply with only the query on a single line, without quotes, links or explanations.",
			"options": map[string]int{"num_predict": 64},
		})
		const unavailable = "AI suggestions are unavailable. You can still paste a YouTube link."
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, endpoint, bytes.NewReader(payload))
		if err != nil {
			http.Error(w, unavailable, http.StatusServiceUnavailable)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			http.Error(w, unavailable, http.StatusServiceUnavailable)
			return
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			http.Error(w, unavailable, http.StatusServiceUnavailable)
			return
		}
		const limit = 32 << 10
		raw, err := io.ReadAll(io.LimitReader(res.Body, limit+1))
		var result struct {
			Response string `json:"response"`
		}
		if err != nil || len(raw) > limit || json.Unmarshal(raw, &result) != nil {
			http.Error(w, unavailable, http.StatusBadGateway)
			return
		}
		query := strings.TrimSpace(strings.Trim(strings.TrimSpace(result.Response), "\""))
		if query == "" || utf8.RuneCountInString(query) > 200 || strings.IndexFunc(query, unicode.IsControl) >= 0 {
			http.Error(w, "Could not generate a search query. Try a different description.", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"query": query})
	}
}
