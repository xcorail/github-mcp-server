package github

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v69/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test for ListDiscussions
func Test_ListDiscussions(t *testing.T) {
	mockDiscussions := []*github.Issue{
		{
			HTMLURL: github.Ptr("https://github.com/owner/repo/discussions/1"),
			ID:      github.Ptr(int64(1)),
			Number:  github.Ptr(1),
			Body:    github.Ptr("Discussion 1 body"),
			State:   github.Ptr("open"),
		},
		{
			HTMLURL: github.Ptr("https://github.com/owner/repo/discussions/2"),
			ID:      github.Ptr(int64(2)),
			Number:  github.Ptr(2),
			Body:    github.Ptr("Discussion 2 body"),
			State:   github.Ptr("closed"),
		},
	}

	mockedClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatchHandler(
			mock.EndpointPattern{
				Pattern: "/repos/owner/repo/discussions",
				Method:  "GET",
			},
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(mockDiscussions)
			}),
		),
	)

	client := github.NewClient(mockedClient)
	_, handler := ListDiscussions(stubGetClientFn(client), translations.NullTranslationHelper)

	request := createMCPRequest(map[string]interface{}{
		"owner": "owner",
		"repo":  "repo",
	})

	result, err := handler(context.Background(), request)
	require.NoError(t, err)

	textContent := getTextResult(t, result)

	var returnedDiscussions []*github.Issue
	err = json.Unmarshal([]byte(textContent.Text), &returnedDiscussions)
	require.NoError(t, err)
	assert.Len(t, returnedDiscussions, len(mockDiscussions))
	for i, discussion := range returnedDiscussions {
		assert.Equal(t, *mockDiscussions[i].ID, *discussion.ID)
		assert.Equal(t, *mockDiscussions[i].HTMLURL, *discussion.HTMLURL)
		assert.Equal(t, *mockDiscussions[i].Body, *discussion.Body)
		assert.Equal(t, *mockDiscussions[i].State, *discussion.State)
	}
}

// Verify tool definition for GetDiscussion
func Test_GetDiscussion(t *testing.T) {
	mockClient := github.NewClient(nil)
	tool, _ := GetDiscussion(stubGetClientFn(mockClient), translations.NullTranslationHelper)

	assert.Equal(t, "get_discussion", tool.Name)
	assert.NotEmpty(t, tool.Description)
	assert.Contains(t, tool.InputSchema.Properties, "owner")
	assert.Contains(t, tool.InputSchema.Properties, "repo")
	assert.Contains(t, tool.InputSchema.Properties, "discussion_id")
	assert.ElementsMatch(t, tool.InputSchema.Required, []string{"owner", "repo", "discussion_id"})

	// Setup mock discussion for success case
	// A GitHub discussion shares the same structure as an issue
	mockDiscussion := &github.Issue{
		HTMLURL: github.Ptr("https://github.com/owner/repo/discussions/1"),
		ID:      github.Ptr(int64(1)),
		Number:  github.Ptr(1),
		Body:    github.Ptr("This is a test discussion"),
		State:   github.Ptr("open"),
		Labels:  []*github.Label{{Name: github.Ptr("newsletter")}},
	}

	tests := []struct {
		name           string
		mockedClient   *http.Client
		requestArgs    map[string]interface{}
		expectError    bool
		expectedResult *github.Issue
		expectedErrMsg string
	}{
		{
			name: "successful discussion retrieval",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.EndpointPattern{
						Pattern: "/repos/owner/repo/discussions/1",
						Method:  "GET",
					},
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusOK)
						_, _ = w.Write([]byte(`{
							"id": 1,
							"html_url": "https://github.com/owner/repo/discussions/1",
							"number": 1,
							"labels": [{"name": "newsletter"}],
							"created_at": "2025-04-25T12:00:00Z",
							"title": "Test Discussion",
							"body": "This is a test discussion",
							"state": "open"
						}`))
					}),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":         "owner",
				"repo":          "repo",
				"discussion_id": int(1),
			},
			expectError:    false,
			expectedResult: mockDiscussion,
		},
		{
			name: "discussion not found",
			mockedClient: mock.NewMockedHTTPClient(
				mock.WithRequestMatchHandler(
					mock.EndpointPattern{
						Pattern: "/repos/owner/repo/discussions/999",
						Method:  "GET",
					},
					http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.WriteHeader(http.StatusNotFound)
						_, _ = w.Write([]byte(`{"message": "Discussion not found"}`))
					}),
				),
			),
			requestArgs: map[string]interface{}{
				"owner":         "owner",
				"repo":          "repo",
				"discussion_id": int(999),
			},
			expectError:    true,
			expectedErrMsg: "failed to get discussion",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup client with mock
			client := github.NewClient(tc.mockedClient)
			_, handler := GetDiscussion(stubGetClientFn(client), translations.NullTranslationHelper)

			// Create call request
			request := createMCPRequest(tc.requestArgs)

			// Call handler
			result, err := handler(context.Background(), request)

			// Verify results
			if tc.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErrMsg)
				return
			}

			require.NoError(t, err)
			textContent := getTextResult(t, result)

			// Unmarshal and verify the result
			var returnedDiscussion github.Issue
			err = json.Unmarshal([]byte(textContent.Text), &returnedDiscussion)
			require.NoError(t, err)
			assert.Equal(t, *tc.expectedResult.ID, *returnedDiscussion.ID)
			assert.Equal(t, *tc.expectedResult.HTMLURL, *returnedDiscussion.HTMLURL)
			assert.Equal(t, *tc.expectedResult.Body, *returnedDiscussion.Body)
			assert.Equal(t, *tc.expectedResult.State, *returnedDiscussion.State)
		})
	}
}

// Test for GetDiscussionComments
func Test_GetDiscussionComments(t *testing.T) {
	mockComments := []*github.IssueComment{
		{
			ID:   github.Ptr(int64(1)),
			Body: github.Ptr("This is the first comment"),
		},
		{
			ID:   github.Ptr(int64(2)),
			Body: github.Ptr("This is the second comment"),
		},
	}

	mockedClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatchHandler(
			mock.EndpointPattern{
				Pattern: "/repos/owner/repo/discussions/1/comments",
				Method:  "GET",
			},
			http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(mockComments)
			}),
		),
	)

	client := github.NewClient(mockedClient)
	_, handler := GetDiscussionComments(stubGetClientFn(client), translations.NullTranslationHelper)

	request := createMCPRequest(map[string]interface{}{
		"owner":         "owner",
		"repo":          "repo",
		"discussion_id": 1,
	})

	result, err := handler(context.Background(), request)
	require.NoError(t, err)

	textContent := getTextResult(t, result)

	var returnedComments []*github.IssueComment
	err = json.Unmarshal([]byte(textContent.Text), &returnedComments)
	require.NoError(t, err)
	assert.Len(t, returnedComments, len(mockComments))
	for i, comment := range returnedComments {
		assert.Equal(t, mockComments[i].ID, comment.ID)
		assert.Equal(t, mockComments[i].Body, comment.Body)
	}
}
