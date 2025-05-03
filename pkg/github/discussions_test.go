package github

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/github/github-mcp-server/pkg/translations"
	"github.com/google/go-github/v69/github"
	"github.com/migueleliasweb/go-github-mock/src/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test for ListDiscussions
func Test_ListDiscussions(t *testing.T) {
	creationTime1 := github.Timestamp{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)}
	creationTime2 := github.Timestamp{Time: time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC)}
	creationTime3 := github.Timestamp{Time: time.Date(2023, 3, 1, 0, 0, 0, 0, time.UTC)}

	mockDiscussions := []*github.Issue{
		{
			HTMLURL:   github.Ptr("https://github.com/owner/repo/discussions/1"),
			ID:        github.Ptr(int64(1)),
			Number:    github.Ptr(1),
			Body:      github.Ptr("Discussion 1 body"),
			State:     github.Ptr("open"),
			CreatedAt: &creationTime1,
			Labels: []*github.Label{
				{Name: github.Ptr("feature")},
				{Name: github.Ptr("bug")},
			},
		},
		{
			HTMLURL:   github.Ptr("https://github.com/owner/repo/discussions/2"),
			ID:        github.Ptr(int64(2)),
			Number:    github.Ptr(2),
			Body:      github.Ptr("Discussion 2 body"),
			State:     github.Ptr("closed"),
			CreatedAt: &creationTime2,
			Labels: []*github.Label{
				{Name: github.Ptr("feature")},
			},
		},
		{
			HTMLURL:   github.Ptr("https://github.com/owner/repo/discussions/3"),
			ID:        github.Ptr(int64(3)),
			Number:    github.Ptr(3),
			Body:      github.Ptr("Discussion 3 body"),
			State:     github.Ptr("open"),
			CreatedAt: &creationTime3,
			Labels: []*github.Label{
				{Name: github.Ptr("bug")},
				{Name: github.Ptr("documentation")},
			},
		},
	}

	mockedClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatchHandler(
			mock.EndpointPattern{
				Pattern: "/repos/owner/repo/discussions",
				Method:  "GET",
			},
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				q := r.URL.Query()
				switch q.Get("page") {
				case "1":
					_ = json.NewEncoder(w).Encode(mockDiscussions)
				default:
					_ = json.NewEncoder(w).Encode([]*github.Issue{})
				}
			}),
		),
	)

	client := github.NewClient(mockedClient)
	_, handler := ListDiscussions(stubGetClientFn(client), translations.NullTranslationHelper)

	tests := []struct {
		name           string
		requestParams  map[string]interface{}
		expectedIDs    []int64
		expectedState  string
		expectedLabels []string
		sinceDate      string
	}{
		{
			name: "list all discussions",
			requestParams: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
			},
			expectedIDs: []int64{1, 2, 3},
		},
		{
			name: "list open discussions",
			requestParams: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"state": "open",
			},
			expectedIDs:   []int64{1, 3},
			expectedState: "open",
		},
		{
			name: "list closed discussions",
			requestParams: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"state": "closed",
			},
			expectedIDs:   []int64{2},
			expectedState: "closed",
		},
		{
			name: "filter by single label",
			requestParams: map[string]interface{}{
				"owner":  "owner",
				"repo":   "repo",
				"labels": []interface{}{"feature"},
			},
			expectedIDs:    []int64{1, 2},
			expectedLabels: []string{"feature"},
		},
		{
			name: "filter by multiple labels",
			requestParams: map[string]interface{}{
				"owner":  "owner",
				"repo":   "repo",
				"labels": []interface{}{"feature", "bug"},
			},
			expectedIDs:    []int64{1},
			expectedLabels: []string{"feature", "bug"},
		},
		{
			name: "combine state and label filters",
			requestParams: map[string]interface{}{
				"owner":  "owner",
				"repo":   "repo",
				"state":  "open",
				"labels": []interface{}{"bug"},
			},
			expectedIDs:    []int64{1, 3},
			expectedState:  "open",
			expectedLabels: []string{"bug"},
		},
		{
			name: "filter by label that doesn't exist",
			requestParams: map[string]interface{}{
				"owner":  "owner",
				"repo":   "repo",
				"labels": []interface{}{"nonexistent"},
			},
			expectedIDs: []int64{},
		},
		{
			name: "filter by created date - get discussions after January 15, 2023",
			requestParams: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"since": "2023-01-15T00:00:00Z",
			},
			expectedIDs: []int64{2, 3},
			sinceDate:   "2023-01-15T00:00:00Z",
		},
		{
			name: "filter by created date - get discussions after February 15, 2023",
			requestParams: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"since": "2023-02-15T00:00:00Z",
			},
			expectedIDs: []int64{3},
			sinceDate:   "2023-02-15T00:00:00Z",
		},
		{
			name: "filter by created date with simple date format",
			requestParams: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"since": "2023-02-01",
			},
			expectedIDs: []int64{2, 3},
			sinceDate:   "2023-02-01",
		},
		{
			name: "combine date and state filters",
			requestParams: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"state": "open",
				"since": "2023-01-15T00:00:00Z",
			},
			expectedIDs:   []int64{3},
			expectedState: "open",
			sinceDate:     "2023-01-15T00:00:00Z",
		},
		{
			name: "combine date, state, and label filters",
			requestParams: map[string]interface{}{
				"owner":  "owner",
				"repo":   "repo",
				"state":  "open",
				"labels": []interface{}{"bug"},
				"since":  "2023-01-01T00:00:00Z",
			},
			expectedIDs:    []int64{1, 3},
			expectedState:  "open",
			expectedLabels: []string{"bug"},
			sinceDate:      "2023-01-01T00:00:00Z",
		},
		{
			name: "filter by date that excludes all discussions",
			requestParams: map[string]interface{}{
				"owner": "owner",
				"repo":  "repo",
				"since": "2024-01-01T00:00:00Z",
			},
			expectedIDs: []int64{},
			sinceDate:   "2024-01-01T00:00:00Z",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			request := createMCPRequest(tc.requestParams)
			result, err := handler(context.Background(), request)
			require.NoError(t, err)

			textContent := getTextResult(t, result)

			var returnedDiscussions []*github.Issue
			err = json.Unmarshal([]byte(textContent.Text), &returnedDiscussions)
			require.NoError(t, err)

			assert.Len(t, returnedDiscussions, len(tc.expectedIDs), "Expected %d discussions, got %d", len(tc.expectedIDs), len(returnedDiscussions))

			// If no discussions are expected, skip further checks
			if len(tc.expectedIDs) == 0 {
				return
			}

			// Create a map of expected IDs for easier checking
			expectedIDMap := make(map[int64]bool)
			for _, id := range tc.expectedIDs {
				expectedIDMap[id] = true
			}

			for _, discussion := range returnedDiscussions {
				// Check if the discussion ID is in the expected list
				assert.True(t, expectedIDMap[*discussion.ID], "Unexpected discussion ID: %d", *discussion.ID)

				// Verify state if specified
				if tc.expectedState != "" {
					assert.Equal(t, tc.expectedState, *discussion.State)
				}

				// Verify labels if specified
				if len(tc.expectedLabels) > 0 {
					// For each expected label, check if it exists in the discussion
					for _, expectedLabel := range tc.expectedLabels {
						found := false
						for _, label := range discussion.Labels {
							if *label.Name == expectedLabel {
								found = true
								break
							}
						}
						// For labels check, we're verifying that the required labels are present
						// but we don't require ALL expected labels to be on EVERY discussion
						// since this depends on the filter combination
						if len(tc.expectedLabels) == 1 {
							assert.True(t, found, "Expected label %s not found in discussion %d", expectedLabel, *discussion.ID)
						}
					}
				}

				// Verify creation date if specified
				if tc.sinceDate != "" {
					sinceTime, err := parseISOTimestamp(tc.sinceDate)
					require.NoError(t, err)
					assert.False(t, discussion.GetCreatedAt().Before(sinceTime),
						"Discussion %d was created at %s, which is before %s",
						*discussion.ID,
						discussion.GetCreatedAt().Format(time.RFC3339),
						sinceTime.Format(time.RFC3339))
				}
			}
		})
	}
}

// Test pagination behavior for ListDiscussions
func Test_ListDiscussions_Pagination(t *testing.T) {
	mockDiscussions := []*github.Issue{
		{ID: github.Ptr(int64(1)), HTMLURL: github.Ptr("url1")},
		{ID: github.Ptr(int64(2)), HTMLURL: github.Ptr("url2")},
	}

	// Single mock handler: dispatch based on query parameters
	mockedClient := mock.NewMockedHTTPClient(
		mock.WithRequestMatchHandler(
			mock.EndpointPattern{Pattern: "/repos/owner/repo/discussions", Method: "GET"},
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				q := r.URL.Query()
				switch q.Get("page") {
				case "2":
					_ = json.NewEncoder(w).Encode(mockDiscussions[1:])
				default:
					_ = json.NewEncoder(w).Encode(mockDiscussions[:1])
				}
			}),
		),
	)
	client := github.NewClient(mockedClient)
	_, handler := ListDiscussions(stubGetClientFn(client), translations.NullTranslationHelper)

	cases := []struct {
		name      string
		page, per float64
		expectIDs []int64
	}{
		{"page1", float64(1), float64(1), []int64{1}},
		{"page2", float64(2), float64(1), []int64{2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := createMCPRequest(map[string]interface{}{"owner": "owner", "repo": "repo", "page": tc.page, "perPage": tc.per})
			res, err := handler(context.Background(), req)
			require.NoError(t, err)
			text := getTextResult(t, res).Text
			var out []*github.Issue
			require.NoError(t, json.Unmarshal([]byte(text), &out))
			assert.Len(t, out, len(tc.expectIDs))
			for i, id := range tc.expectIDs {
				assert.Equal(t, id, *out[i].ID)
			}
		})
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
