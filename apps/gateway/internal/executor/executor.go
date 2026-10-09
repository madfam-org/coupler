package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/madfam-org/coupler/apps/gateway/internal/janua"
	"github.com/madfam-org/coupler/apps/gateway/internal/registry"
)

type Request struct {
	Tool         string
	Arguments    map[string]any
	ConnectionID string
	ActingUserID string
	UserJWT      string
}

type Result struct {
	Tool      string         `json:"tool"`
	Connector string         `json:"connector"`
	Output    map[string]any `json:"output"`
}

type Executor struct {
	janua  *janua.Client
	client *http.Client
}

func New() *Executor {
	return &Executor{
		janua:  janua.NewClient(),
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (e *Executor) Execute(ctx context.Context, tool registry.Tool, req Request) (Result, error) {
	connID, err := e.janua.ResolveConnectionID(ctx, req.ActingUserID, tool.Connector, req.ConnectionID, req.UserJWT)
	if err != nil {
		return Result{}, err
	}
	delegation, err := e.janua.DelegateToken(ctx, connID, req.ActingUserID, 300)
	if err != nil {
		return Result{}, err
	}

	switch tool.Name {
	case "coupler.github.list_repos":
		out, err := e.githubListRepos(ctx, delegation.AccessToken, req.Arguments)
		return Result{Tool: tool.Name, Connector: tool.Connector, Output: out}, err
	case "coupler.github.get_issue":
		out, err := e.githubGetIssue(ctx, delegation.AccessToken, req.Arguments)
		return Result{Tool: tool.Name, Connector: tool.Connector, Output: out}, err
	case "coupler.github.create_issue":
		out, err := e.githubCreateIssue(ctx, delegation.AccessToken, req.Arguments)
		return Result{Tool: tool.Name, Connector: tool.Connector, Output: out}, err
	case "coupler.slack.post_message":
		out, err := e.slackPostMessage(ctx, delegation.AccessToken, req.Arguments)
		return Result{Tool: tool.Name, Connector: tool.Connector, Output: out}, err
	case "coupler.slack.list_channels":
		out, err := e.slackListChannels(ctx, delegation.AccessToken, req.Arguments)
		return Result{Tool: tool.Name, Connector: tool.Connector, Output: out}, err
	default:
		return Result{}, fmt.Errorf("no runtime handler for %s", tool.Name)
	}
}

func (e *Executor) githubListRepos(ctx context.Context, token string, args map[string]any) (map[string]any, error) {
	visibility := "all"
	if v, ok := args["visibility"].(string); ok && v != "" {
		visibility = v
	}
	u := "https://api.github.com/user/repos?per_page=100&visibility=" + url.QueryEscape(visibility)
	return e.githubGET(ctx, token, u)
}

func (e *Executor) githubGetIssue(ctx context.Context, token string, args map[string]any) (map[string]any, error) {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	number := intArg(args, "number")
	if owner == "" || repo == "" || number == 0 {
		return nil, fmt.Errorf("owner, repo, and number required")
	}
	u := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d", owner, repo, number)
	return e.githubGET(ctx, token, u)
}

func (e *Executor) githubCreateIssue(ctx context.Context, token string, args map[string]any) (map[string]any, error) {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	title, _ := args["title"].(string)
	if owner == "" || repo == "" || title == "" {
		return nil, fmt.Errorf("owner, repo, and title required")
	}
	body := map[string]any{"title": title}
	if b, ok := args["body"].(string); ok {
		body["body"] = b
	}
	u := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", owner, repo)
	return e.githubPOST(ctx, token, u, body)
}

func (e *Executor) githubGET(ctx context.Context, token, u string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github %d: %s", resp.StatusCode, string(data))
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return map[string]any{"data": out}, nil
}

func (e *Executor) githubPOST(ctx context.Context, token, u string, payload map[string]any) (map[string]any, error) {
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("github %d: %s", resp.StatusCode, string(data))
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return map[string]any{"data": out}, nil
}

func (e *Executor) slackPostMessage(ctx context.Context, token string, args map[string]any) (map[string]any, error) {
	channel, _ := args["channel"].(string)
	text, _ := args["text"].(string)
	if channel == "" || text == "" {
		return nil, fmt.Errorf("channel and text required")
	}
	payload := map[string]any{"channel": channel, "text": text}
	if ts, ok := args["thread_ts"].(string); ok && ts != "" {
		payload["thread_ts"] = ts
	}
	return e.slackAPI(ctx, token, "chat.postMessage", payload)
}

func (e *Executor) slackListChannels(ctx context.Context, token string, args map[string]any) (map[string]any, error) {
	limit := intArg(args, "limit")
	if limit <= 0 {
		limit = 100
	}
	return e.slackAPI(ctx, token, "conversations.list", map[string]any{
		"types": "public_channel,private_channel",
		"limit": limit,
	})
}

func (e *Executor) slackAPI(ctx context.Context, token, method string, payload map[string]any) (map[string]any, error) {
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://slack.com/api/"+method, strings.NewReader(string(raw)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	if ok, _ := out["ok"].(bool); !ok {
		return out, fmt.Errorf("slack error: %v", out["error"])
	}
	return out, nil
}

func intArg(args map[string]any, key string) int {
	switch v := args[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}
