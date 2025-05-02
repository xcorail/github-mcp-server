package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v64/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Helpers implemented as GitHub API calls, as discussions are not available in the Go client library

func ghAPIListDiscussions(ctx context.Context, client *http.Client, owner, repo string, page, perPage int) ([]*github.Issue, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/discussions?page=%d&per_page=%d", owner, repo, page, perPage)
	// Create a new HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute the HTTP request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for a successful response
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	// Decode the response body
	var discussions []*github.Issue
	if err := json.NewDecoder(resp.Body).Decode(&discussions); err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", err)
	}

	return discussions, nil
}

func ghAPIGetDiscussion(ctx context.Context, client *http.Client, owner, repo string, discussionID int) (*github.Issue, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/discussions/%d", owner, repo, discussionID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("unexpected status code: %d, failed to read response body", resp.StatusCode)
		}
		return nil, fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode, string(body))
	}

	var discussion github.Issue
	if err := json.NewDecoder(resp.Body).Decode(&discussion); err != nil {
		return nil, err
	}

	return &discussion, nil
}

func ghAPIGetDiscussionComments(ctx context.Context, client *http.Client, owner, repo string, discussionID int) ([]*github.IssueComment, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/discussions/%d/comments", owner, repo, discussionID)

	// Create a new HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	// Execute the HTTP request
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check for a successful response
	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("unexpected status code: %d, failed to read response body", resp.StatusCode)
		}
		return nil, fmt.Errorf("unexpected status code: %d, response: %s", resp.StatusCode, string(body))
	}

	// Decode the response body
	var comments []*github.IssueComment
	if err := json.NewDecoder(resp.Body).Decode(&comments); err != nil {
		return nil, fmt.Errorf("failed to decode response body: %w", err)
	}

	return comments, nil
}

// MCP Toolset functions
// These functions are used to create MCP tools for GitHub discussions

func ListDiscussions(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("list_discussions",
			mcp.WithDescription(t("TOOL_LIST_DISCUSSIONS_DESCRIPTION", "List discussions for a repository")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_LIST_DISCUSSIONS_USER_TITLE", "List discussions"),
				ReadOnlyHint: true,
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			WithPagination(),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			// Convert *github.Client to *http.Client
			httpClient := client.Client()

			// Extract pagination parameters
			var page, perPage int
			if p, ok := request.Params.Arguments["page"].(float64); ok {
				page = int(p)
			}
			if pp, ok := request.Params.Arguments["perPage"].(float64); ok {
				perPage = int(pp)
			}

			discussions, err := ghAPIListDiscussions(ctx, httpClient, owner, repo, page, perPage)
			if err != nil {
				return nil, fmt.Errorf("failed to list discussions: %w", err)
			}

			r, err := json.Marshal(discussions)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal discussions: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

func GetDiscussion(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_discussion",
			mcp.WithDescription(t("TOOL_GET_DISCUSSION_DESCRIPTION", "Get a specific discussion by ID")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_DISCUSSION_USER_TITLE", "Get discussion"),
				ReadOnlyHint: true,
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithNumber("discussion_id",
				mcp.Required(),
				mcp.Description("Discussion ID"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			discussionID, err := requiredParam[int](request, "discussion_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			discussion, err := ghAPIGetDiscussion(ctx, client.Client(), owner, repo, discussionID)
			if err != nil {
				return nil, fmt.Errorf("failed to get discussion: %w", err)
			}

			r, err := json.Marshal(discussion)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal discussion: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}

func GetDiscussionComments(getClient GetClientFn, t translations.TranslationHelperFunc) (tool mcp.Tool, handler server.ToolHandlerFunc) {
	return mcp.NewTool("get_discussion_comments",
			mcp.WithDescription(t("TOOL_GET_DISCUSSION_COMMENTS_DESCRIPTION", "Get comments from a discussion")),
			mcp.WithToolAnnotation(mcp.ToolAnnotation{
				Title:        t("TOOL_GET_DISCUSSION_COMMENTS_USER_TITLE", "Get discussion comments"),
				ReadOnlyHint: true,
			}),
			mcp.WithString("owner",
				mcp.Required(),
				mcp.Description("Repository owner"),
			),
			mcp.WithString("repo",
				mcp.Required(),
				mcp.Description("Repository name"),
			),
			mcp.WithNumber("discussion_id",
				mcp.Required(),
				mcp.Description("Discussion ID"),
			),
		),
		func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			owner, err := requiredParam[string](request, "owner")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			repo, err := requiredParam[string](request, "repo")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			discussionID, err := requiredParam[int](request, "discussion_id")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			comments, err := ghAPIGetDiscussionComments(ctx, client.Client(), owner, repo, discussionID)
			if err != nil {
				return nil, fmt.Errorf("failed to get discussion comments: %w", err)
			}

			r, err := json.Marshal(comments)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal comments: %w", err)
			}

			return mcp.NewToolResultText(string(r)), nil
		}
}
