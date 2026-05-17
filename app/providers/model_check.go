package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"ai-gateway/config"
	"ai-gateway/types"
)

const upstreamModelsCheckTimeout = 25 * time.Second

// UpstreamModelCheckResult is one provider's outcome from GET {base_url}/models.
type UpstreamModelCheckResult struct {
	Provider       string `json:"provider"`
	OK             bool   `json:"ok"`
	ModelCount     int    `json:"model_count,omitempty"`
	HTTPStatus     int    `json:"http_status,omitempty"`
	Error          string `json:"error,omitempty"`
	ResponseTimeMs int64  `json:"response_time_ms"`
}

// CheckUpstreamModels calls each provider's OpenAI-style models endpoint in parallel.
func CheckUpstreamModels(ctx context.Context, provs []config.Provider) []UpstreamModelCheckResult {
	results := make([]UpstreamModelCheckResult, len(provs))
	var wg sync.WaitGroup
	for i := range provs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = checkUpstreamModelList(ctx, provs[i])
		}(i)
	}
	wg.Wait()
	return results
}

func checkUpstreamModelList(ctx context.Context, p config.Provider) (out UpstreamModelCheckResult) {
	out.Provider = p.Name
	start := time.Now()
	defer func() {
		out.ResponseTimeMs = time.Since(start).Milliseconds()
	}()

	reqCtx, cancel := context.WithTimeout(ctx, upstreamModelsCheckTimeout)
	defer cancel()

	url := strings.TrimRight(p.BaseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		out.Error = fmt.Sprintf("create request: %v", err)
		return out
	}
	req.Header.Set("Authorization", "Bearer "+p.APIKey)

	client := &http.Client{Timeout: upstreamModelsCheckTimeout}
	resp, err := client.Do(req)
	if err != nil {
		out.Error = fmt.Sprintf("request failed: %v", err)
		return out
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		out.Error = fmt.Sprintf("read body: %v", readErr)
		return out
	}

	out.HTTPStatus = resp.StatusCode
	if resp.StatusCode != http.StatusOK {
		out.Error = fmt.Sprintf("HTTP %d %s", resp.StatusCode, truncateBody(string(body), 200))
		return out
	}

	var list types.ModelsResponse
	if err := json.Unmarshal(body, &list); err != nil {
		out.Error = fmt.Sprintf("invalid JSON: %v", err)
		return out
	}

	out.OK = true
	out.ModelCount = len(list.Data)
	return out
}

func truncateBody(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	count := 0
	for i := range s {
		if count == maxRunes {
			return s[:i] + "..."
		}
		count++
	}
	return s
}
