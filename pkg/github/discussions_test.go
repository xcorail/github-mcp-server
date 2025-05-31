package github

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/github/github-mcp-server/internal/githubv4mock"
	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v69/github"
	"github.com/shurcooL/githubv4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	discussionsAll = []map[string]any{
		{"number": 1, "title": "Discussion 1 title", "createdAt": "2023-01-01T00:00:00Z", "category": map[string]any{"name": "news"}, "url": "https://github.com/owner/repo/discussions/1"},
		{"number": 2, "title": "Discussion 2 title", "createdAt": "2023-02-01T00:00:00Z", "category": map[string]any{"name": "updates"}, "url": "https://github.com/owner/repo/discussions/2"},
		{"number": 3, "title": "Discussion 3 title", "createdAt": "2023-03-01T00:00:00Z", "category": map[string]any{"name": "questions"}, "url": "https://github.com/owner/repo/discussions/3"},
	}
	mockResponseListAll = githubv4mock.DataResponse(map[string]any{
		"repository": map[string]any{
			"discussions": map[string]any{"nodes": discussionsAll},
		},
	})
	mockResponseCategory = githubv4mock.DataResponse(map[string]any{
		"repository": map[string]any{
			"discussions": map[string]any{"nodes": discussionsAll[:1]}, // Only return the first discussion for category test
		},
	})
	mockErrorRepoNotFound = githubv4mock.ErrorResponse("repository not found")
)

func Test_ListDiscussions(t *testing.T) {
	// Verify tool definition and schema
	toolDef, _ := ListDiscussions(nil, translations.NullTranslationHelper)
	assert.Equal(t, "list_discussions", toolDef.Name)
	assert.NotEmpty(t, toolDef.Description)
	assert.Contains(t, toolDef.InputSchema.Properties, "owner")
	assert.Contains(t, toolDef.InputSchema.Properties, "repo")
	assert.ElementsMatch(t, toolDef.InputSchema.Required, []string{"owner", "repo"})

	// GraphQL query struct
	var q struct {
		Repository struct {
			Discussions struct {
				Nodes []struct {
					Number    githubv4.Int
					Title     githubv4.String
					CreatedAt githubv4.DateTime
					Category  struct {
						Name githubv4.String
					} `graphql:"category"`
					URL githubv4.String `graphql:"url"`
				}
			} `graphql:"discussions(categoryId: $categoryId, orderBy: {field: $sort, direction: $direction}, first: $first, after: $after)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}

	varsListAll := map[string]interface{}{
		"owner":      githubv4.String("owner"),
		"repo":       githubv4.String("repo"),
		"categoryId": githubv4.ID(""),
		"sort":       githubv4.DiscussionOrderField(""),
		"direction":  githubv4.OrderDirection(""),
		"first":      githubv4.Int(0),
		"after":      githubv4.String(""),
	}

	varsListInvalid := map[string]interface{}{
		"owner":      githubv4.String("invalid"),
		"repo":       githubv4.String("repo"),
		"categoryId": githubv4.ID(""),
		"sort":       githubv4.DiscussionOrderField(""),
		"direction":  githubv4.OrderDirection(""),
		"first":      githubv4.Int(0),
		"after":      githubv4.String(""),
	}

	varsListWithCategory := map[string]interface{}{
		"owner":      githubv4.String("owner"),
		"repo":       githubv4.String("repo"),
		"categoryId": githubv4.ID("12345"),
		"sort":       githubv4.DiscussionOrderField(""),
		"direction":  githubv4.OrderDirection(""),
		"first":      githubv4.Int(0),
		"after":      githubv4.String(""),
	}

	tests := []struct {
		name        string
		vars        map[string]interface{}
		reqParams   map[string]interface{}
		response    githubv4mock.GQLResponse
		expectError bool
		expectedIds []int64
		errContains string
	}{
		{
			name: "list all discussions",
			vars: varsListAll,
			reqParams: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
			},
			response:    mockResponseListAll,
			expectError: false,
			expectedIds: []int64{1, 2, 3},
		},
		{
			name: "invalid owner or repo",
			vars: varsListInvalid,
			reqParams: map[string]interface{}{
				"owner": "invalid",
				"repo":  "repo",
			},
			response:    mockErrorRepoNotFound,
			expectError: true,
			errContains: "repository not found",
		},
		{
			name: "list discussions with category",
			vars: varsListWithCategory,
			reqParams: map[string]interface{}{
				"owner":      "owner",
				"repo":       "repo",
				"categoryId": "12345",
			},
			response:    mockResponseCategory,
			expectError: false,
			expectedIds: []int64{1},
		},
		{
			name: "list discussions with since date",
			vars: varsListAll,
			reqParams: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"since": "2023-01-10T00:00:00Z",
			},
			response:    mockResponseListAll,
			expectError: false,
			expectedIds: []int64{2, 3},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matcher := githubv4mock.NewQueryMatcher(q, tc.vars, tc.response)
			httpClient := githubv4mock.NewMockedHTTPClient(matcher)
			gqlClient := githubv4.NewClient(httpClient)
			_, handler := ListDiscussions(stubGetGQLClientFn(gqlClient), translations.NullTranslationHelper)

			req := createMCPRequest(tc.reqParams)
			res, err := handler(context.Background(), req)
			text := getTextResult(t, res).Text

			if tc.expectError {
				require.True(t, res.IsError)
				assert.Contains(t, text, tc.errContains)
				return
			}
			require.NoError(t, err)

			var returnedDiscussions []*github.Issue
			err = json.Unmarshal([]byte(text), &returnedDiscussions)
			require.NoError(t, err)

			assert.Len(t, returnedDiscussions, len(tc.expectedIds), "Expected %d discussions, got %d", len(tc.expectedIds), len(returnedDiscussions))

			// If no discussions are expected, skip further checks
			if len(tc.expectedIds) == 0 {
				return
			}

			// Create a map of expected IDs for easier checking
			expectedIDMap := make(map[int64]bool)
			for _, id := range tc.expectedIds {
				expectedIDMap[id] = true
			}

			for _, discussion := range returnedDiscussions {
				// Check if the discussion Number is in the expected list
				assert.True(t, expectedIDMap[int64(*discussion.Number)], "Unexpected discussion Number: %d", *discussion.Number)
			}
		})
	}
}

func Test_GetDiscussion(t *testing.T) {
	// Verify tool definition and schema
	toolDef, _ := GetDiscussion(nil, translations.NullTranslationHelper)
	assert.Equal(t, "get_discussion", toolDef.Name)
	assert.NotEmpty(t, toolDef.Description)
	assert.Contains(t, toolDef.InputSchema.Properties, "owner")
	assert.Contains(t, toolDef.InputSchema.Properties, "repo")
	assert.Contains(t, toolDef.InputSchema.Properties, "discussion_id")
	assert.ElementsMatch(t, toolDef.InputSchema.Required, []string{"owner", "repo", "discussion_id"})

	var q struct {
		Repository struct {
			Discussion struct {
				Number    githubv4.Int
				Body      githubv4.String
				State     githubv4.String
				CreatedAt githubv4.DateTime
				URL       githubv4.String `graphql:"url"`
			} `graphql:"discussion(number: $discussionID)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}
	vars := map[string]interface{}{
		"owner":        githubv4.String("owner"),
		"repo":         githubv4.String("repo"),
		"discussionID": githubv4.Int(1),
	}
	tests := []struct {
		name        string
		response    githubv4mock.GQLResponse
		expectError bool
		expected    *github.Issue
		errContains string
	}{
		{
			name: "successful retrieval",
			response: githubv4mock.DataResponse(map[string]any{
				"repository": map[string]any{"discussion": map[string]any{
					"number":    1,
					"body":      "This is a test discussion",
					"state":     "open",
					"url":       "https://github.com/owner/repo/discussions/1",
					"createdAt": "2025-04-25T12:00:00Z",
				}},
			}),
			expectError: false,
			expected: &github.Issue{
				HTMLURL:   github.Ptr("https://github.com/owner/repo/discussions/1"),
				Number:    github.Ptr(1),
				Body:      github.Ptr("This is a test discussion"),
				State:     github.Ptr("open"),
				CreatedAt: &github.Timestamp{Time: time.Date(2025, 4, 25, 12, 0, 0, 0, time.UTC)},
			},
		},
		{
			name:        "discussion not found",
			response:    githubv4mock.ErrorResponse("discussion not found"),
			expectError: true,
			errContains: "discussion not found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matcher := githubv4mock.NewQueryMatcher(q, vars, tc.response)
			httpClient := githubv4mock.NewMockedHTTPClient(matcher)
			gqlClient := githubv4.NewClient(httpClient)
			_, handler := GetDiscussion(stubGetGQLClientFn(gqlClient), translations.NullTranslationHelper)

			req := createMCPRequest(map[string]interface{}{"owner": "owner", "repo": "repo", "discussion_id": float64(1)})
			res, err := handler(context.Background(), req)
			text := getTextResult(t, res).Text

			if tc.expectError {
				require.True(t, res.IsError)
				assert.Contains(t, text, tc.errContains)
				return
			}

			require.NoError(t, err)
			var out github.Issue
			require.NoError(t, json.Unmarshal([]byte(text), &out))
			assert.Equal(t, *tc.expected.HTMLURL, *out.HTMLURL)
			assert.Equal(t, *tc.expected.Number, *out.Number)
			assert.Equal(t, *tc.expected.Body, *out.Body)
			assert.Equal(t, *tc.expected.State, *out.State)
		})
	}
}

func Test_GetDiscussionComments(t *testing.T) {
	// Verify tool definition and schema
	toolDef, _ := GetDiscussionComments(nil, translations.NullTranslationHelper)
	assert.Equal(t, "get_discussion_comments", toolDef.Name)
	assert.NotEmpty(t, toolDef.Description)
	assert.Contains(t, toolDef.InputSchema.Properties, "owner")
	assert.Contains(t, toolDef.InputSchema.Properties, "repo")
	assert.Contains(t, toolDef.InputSchema.Properties, "discussion_id")
	assert.ElementsMatch(t, toolDef.InputSchema.Required, []string{"owner", "repo", "discussion_id"})

	var q struct {
		Repository struct {
			Discussion struct {
				Comments struct {
					Nodes []struct {
						Body githubv4.String
					}
				} `graphql:"comments(first:100)"`
			} `graphql:"discussion(number: $discussionID)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}
	vars := map[string]interface{}{
		"owner":        githubv4.String("owner"),
		"repo":         githubv4.String("repo"),
		"discussionID": githubv4.Int(1),
	}
	mockResponse := githubv4mock.DataResponse(map[string]any{
		"repository": map[string]any{
			"discussion": map[string]any{
				"comments": map[string]any{
					"nodes": []map[string]any{
						{"body": "This is the first comment"},
						{"body": "This is the second comment"},
					},
				},
			},
		},
	})
	matcher := githubv4mock.NewQueryMatcher(q, vars, mockResponse)
	httpClient := githubv4mock.NewMockedHTTPClient(matcher)
	gqlClient := githubv4.NewClient(httpClient)
	_, handler := GetDiscussionComments(stubGetGQLClientFn(gqlClient), translations.NullTranslationHelper)

	request := createMCPRequest(map[string]interface{}{
		"owner":         "owner",
		"repo":          "repo",
		"discussion_id": float64(1),
	})

	result, err := handler(context.Background(), request)
	require.NoError(t, err)

	textContent := getTextResult(t, result)

	var returnedComments []*github.IssueComment
	err = json.Unmarshal([]byte(textContent.Text), &returnedComments)
	require.NoError(t, err)
	assert.Len(t, returnedComments, 2)
	expectedBodies := []string{"This is the first comment", "This is the second comment"}
	for i, comment := range returnedComments {
		assert.Equal(t, expectedBodies[i], *comment.Body)
	}
}

func Test_ListDiscussionCategories(t *testing.T) {
	var q struct {
		Repository struct {
			DiscussionCategories struct {
				Nodes []struct {
					ID   githubv4.ID
					Name githubv4.String
				}
			} `graphql:"discussionCategories(first: 30)"`
		} `graphql:"repository(owner: $owner, name: $repo)"`
	}
	vars := map[string]interface{}{
		"owner": githubv4.String("owner"),
		"repo":  githubv4.String("repo"),
	}
	mockResp := githubv4mock.DataResponse(map[string]any{
		"repository": map[string]any{
			"discussionCategories": map[string]any{
				"nodes": []map[string]any{
					{"id": "123", "name": "CategoryOne"},
					{"id": "456", "name": "CategoryTwo"},
				},
			},
		},
	})
	matcher := githubv4mock.NewQueryMatcher(q, vars, mockResp)
	httpClient := githubv4mock.NewMockedHTTPClient(matcher)
	gqlClient := githubv4.NewClient(httpClient)

	tool, handler := ListDiscussionCategories(stubGetGQLClientFn(gqlClient), translations.NullTranslationHelper)
	assert.Equal(t, "list_discussion_categories", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo"})

	request := createMCPRequest(map[string]interface{}{"owner": "owner", "repo": "repo"})
	result, err := handler(context.Background(), request)
	require.NoError(t, err)

	text := getTextResult(t, result).Text
	var categories []map[string]string
	require.NoError(t, json.Unmarshal([]byte(text), &categories))
	assert.Len(t, categories, 2)
	assert.Equal(t, "123", categories[0]["id"])
	assert.Equal(t, "CategoryOne", categories[0]["name"])
	assert.Equal(t, "456", categories[1]["id"])
	assert.Equal(t, "CategoryTwo", categories[1]["name"])
}
