package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.openai.com/v1"
const defaultModel = "gpt-4o-mini"

// Config controls which OpenAI-compatible API to call.
type Config struct {
	APIKey  string
	BaseURL string
	Model   string
	Limit   int
	Timeout time.Duration
}

// FromEnv builds Config from environment variables:
// OPENAI_API_KEY (required), OPENAI_BASE_URL, OPENAI_MODEL.
func FromEnv() (Config, error) {
	key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	if key == "" {
		return Config{}, fmt.Errorf("OPENAI_API_KEY is not set")
	}
	base := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
	if base == "" {
		base = defaultBaseURL
	}
	model := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
	if model == "" {
		model = defaultModel
	}
	return Config{
		APIKey:  key,
		BaseURL: strings.TrimRight(base, "/"),
		Model:   model,
		Limit:   5,
		Timeout: 60 * time.Second,
	}, nil
}

// Client talks to an OpenAI-compatible chat completions API.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

func New(cfg Config) *Client {
	if cfg.Limit <= 0 {
		cfg.Limit = 5
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	if cfg.Model == "" {
		cfg.Model = defaultModel
	}
	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

type chatRequest struct {
	Model          string        `json:"model"`
	Messages       []chatMessage `json:"messages"`
	Temperature    float64       `json:"temperature"`
	ResponseFormat *respFormat   `json:"response_format,omitempty"`
}

type respFormat struct {
	Type string `json:"type"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

type urlPlan struct {
	URLs    []string `json:"urls"`
	Summary string   `json:"summary"`
}

// ProposeURLs asks the model for direct downloadable HTTP(S) links for the prompt.
func (c *Client) ProposeURLs(prompt string) (urls []string, summary string, err error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, "", fmt.Errorf("empty agent prompt")
	}

	system := fmt.Sprintf(`You are a download-planning agent for a concurrent file downloader.
Given a user request, propose up to %d direct, publicly reachable HTTP or HTTPS file URLs.
Prefer stable, well-known sample/demo links when the user is vague (images, PDFs, zip samples, etc.).
Only include URLs that look like direct file downloads (not HTML article pages), when possible.
Respond with JSON only in this shape:
{"summary":"one short sentence","urls":["https://...","https://..."]}`, c.cfg.Limit)

	body, err := json.Marshal(chatRequest{
		Model: c.cfg.Model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: prompt},
		},
		Temperature:    0.2,
		ResponseFormat: &respFormat{Type: "json_object"},
	})
	if err != nil {
		return nil, "", err
	}

	endpoint := c.cfg.BaseURL + "/chat/completions"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("agent request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read agent response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("agent API status %d: %s", resp.StatusCode, truncate(string(raw), 300))
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, "", fmt.Errorf("decode agent response: %w", err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return nil, "", fmt.Errorf("agent API error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, "", fmt.Errorf("agent returned no choices")
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	plan, err := parsePlan(content)
	if err != nil {
		return nil, "", err
	}

	cleaned := sanitizeURLs(plan.URLs, c.cfg.Limit)
	if len(cleaned) == 0 {
		return nil, plan.Summary, fmt.Errorf("agent proposed no valid http(s) URLs")
	}
	return cleaned, plan.Summary, nil
}

func parsePlan(content string) (urlPlan, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var plan urlPlan
	if err := json.Unmarshal([]byte(content), &plan); err == nil && len(plan.URLs) > 0 {
		return plan, nil
	}

	// Fallback: scrape URLs from free-form text.
	re := regexp.MustCompile(`https?://[^\s"'<>]+`)
	found := re.FindAllString(content, -1)
	if len(found) == 0 {
		return urlPlan{}, fmt.Errorf("could not parse URLs from agent output: %s", truncate(content, 200))
	}
	return urlPlan{URLs: found, Summary: "Parsed URLs from agent text"}, nil
}

func sanitizeURLs(in []string, limit int) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		raw = strings.TrimRight(raw, ".,);]")
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			continue
		}
		// Require a real-looking host (has a dot) or localhost.
		host := u.Hostname()
		if host != "localhost" && !strings.Contains(host, ".") {
			continue
		}
		s := u.String()
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
