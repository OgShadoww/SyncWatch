package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSuggestVideo(t *testing.T) {
	const description = "a quiet train ride through the Alps"
	const query = "Swiss Alps scenic train ride"
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Model, Prompt string
			Stream        *bool
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if r.Method != "POST" || r.URL.Path != "/api/generate" || input.Model != "test-model" ||
			input.Prompt != description || input.Stream == nil || *input.Stream {
			t.Errorf("unexpected Ollama request: %s %s %+v", r.Method, r.URL.Path, input)
		}
		json.NewEncoder(w).Encode(map[string]any{"response": query, "done": true})
	}))
	defer ollama.Close()
	t.Setenv("OLLAMA_URL", ollama.URL)
	t.Setenv("OLLAMA_MODEL", "test-model")
	router, _ := New(t.Context(), nil, t.TempDir(), "")
	req := httptest.NewRequest("POST", "/api/ai/suggest", strings.NewReader(`{"description":"`+description+`"}`))
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)
	var got struct{ Query string }
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &got) != nil || got.Query != query {
		t.Fatalf("unexpected suggestion: %d %s", res.Code, res.Body)
	}
}
