package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v64/github"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// Helpers implemented as GitHub API calls, as discussions are not available in the Go client library
// This toolset is deliberately limited to a few basic functions. The plan is to first implement native support in the GitHub go library, then use it for a better and consistent support in the MCP server.

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
			mcp.WithString("state",
				mcp.Description("State filter (open, closed, all)"),
			),
			mcp.WithArray("labels",
				mcp.Description("Filter by labels"),
				mcp.Items(
					map[string]interface{}{
						"type": "string",
					},
				),
			),
			mcp.WithString("since",
				mcp.Description("Filter by date (ISO 8601 timestamp)"),
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

			client, err := getClient(ctx)
			if err != nil {
				return nil, fmt.Errorf("failed to get GitHub client: %w", err)
			}

			// Convert *github.Client to *http.Client
			httpClient := client.Client()

			// Extract pagination parameters requested by the user
			var requestedPage, requestedPerPage int
			if p, ok := request.Params.Arguments["page"].(float64); ok {
				requestedPage = int(p)
			}
			if pp, ok := request.Params.Arguments["perPage"].(float64); ok {
				requestedPerPage = int(pp)
			}

			// Default pagination values if not specified
			if requestedPage <= 0 {
				requestedPage = 1
			}
			if requestedPerPage <= 0 {
				requestedPerPage = 30 // GitHub API default
			}

			// Extract state parameter (default to "all" if not provided)
			state := "all"
			if s, ok := request.Params.Arguments["state"].(string); ok && s != "" {
				state = s
			}

			// Extract labels parameter
			labels, err := OptionalStringArrayParam(request, "labels")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid labels parameter: %v", err)), nil
			}

			// Extract since parameter for created_at filtering
			since, err := OptionalParam[string](request, "since")
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}

			// Parse the timestamp if provided
			var sinceTimestamp time.Time
			if since != "" {
				sinceTimestamp, err = parseISOTimestamp(since)
				if err != nil {
					return mcp.NewToolResultError(fmt.Sprintf("failed to list discussions: %s", err.Error())), nil
				}
			}

			// Determine if we need to fetch all pages for post-filtering
			needAllPages := state != "all" || len(labels) > 0 || since != ""

			// For state filtering, label filtering, or date filtering, we need to retrieve all discussions across multiple pages
			// and then apply pagination to the filtered results
			allDiscussions := []*github.Issue{}

			if needAllPages {
				currentPage := 1
				perPageForFetching := 100 // Maximum allowed by GitHub API

				for {
					pageDiscussions, err := ghAPIListDiscussions(ctx, httpClient, owner, repo, currentPage, perPageForFetching)
					if err != nil {
						return nil, fmt.Errorf("failed to list discussions on page %d: %w", currentPage, err)
					}

					// No more discussions to fetch
					if len(pageDiscussions) == 0 {
						break
					}

					allDiscussions = append(allDiscussions, pageDiscussions...)
					currentPage++

					// Check context cancellation between page fetches
					select {
					case <-ctx.Done():
						return nil, ctx.Err()
					default:
					}
				}

				// Apply filters to all discussions
				filteredDiscussions := make([]*github.Issue, 0, len(allDiscussions))
				for _, discussion := range allDiscussions {
					// Filter by state if specified
					if state != "all" && discussion.GetState() != state {
						continue
					}

					// Filter by labels if specified
					if len(labels) > 0 {
						// Check if discussion has all required labels
						hasAllLabels := true
						discussionLabels := make(map[string]bool)

						// Build a map of the discussion's labels for efficient lookup
						for _, label := range discussion.Labels {
							discussionLabels[*label.Name] = true
						}

						// Check if all required labels are present
						for _, requiredLabel := range labels {
							if !discussionLabels[requiredLabel] {
								hasAllLabels = false
								break
							}
						}

						if !hasAllLabels {
							continue
						}
					}

					// Filter by creation date if specified
					if !sinceTimestamp.IsZero() {
						createdAt := discussion.GetCreatedAt()
						if createdAt.Before(sinceTimestamp) {
							continue
						}
					}

					filteredDiscussions = append(filteredDiscussions, discussion)
				}

				// Apply pagination to filtered results
				startIndex := (requestedPage - 1) * requestedPerPage
				endIndex := startIndex + requestedPerPage

				// Ensure bounds are within array limits
				if startIndex >= len(filteredDiscussions) {
					// Page is beyond available results, return empty array
					filteredDiscussions = []*github.Issue{}
				} else {
					if endIndex > len(filteredDiscussions) {
						endIndex = len(filteredDiscussions)
					}
					filteredDiscussions = filteredDiscussions[startIndex:endIndex]
				}

				allDiscussions = filteredDiscussions
			} else {
				// No filtering, use the standard GitHub API pagination
				discussions, err := ghAPIListDiscussions(ctx, httpClient, owner, repo, requestedPage, requestedPerPage)
				if err != nil {
					return nil, fmt.Errorf("failed to list discussions: %w", err)
				}
				allDiscussions = discussions
			}

			r, err := json.Marshal(allDiscussions)
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
