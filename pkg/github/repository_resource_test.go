package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/github/github-mcp-server/pkg/raw"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// errorTransport is a http.RoundTripper that always returns an error.
type errorTransport struct {
	err error
}

func (t *errorTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, t.err
}

type resourceResponseType int

const (
	resourceResponseTypeUnknown resourceResponseType = iota
	resourceResponseTypeBlob
	resourceResponseTypeText
)

func Test_repositoryResourceContents(t *testing.T) {
	base, _ := url.Parse("https://raw.example.com/")
	tests := []struct {
		name                 string
		mockedClient         *http.Client
		uri                  string
		handlerFn            func() mcp.ResourceHandler
		expectedResponseType resourceResponseType
		expectError          string
		expectedResult       *mcp.ReadResourceResult
	}{
		{
			name: "missing owner",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetRawReposContentsByOwnerByRepoByPath: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "text/markdown")
					_, err := w.Write([]byte("# Test Repository\n\nThis is a test repository."))
					require.NoError(t, err)
				}),
			}),
			uri: "repo:///repo/contents/README.md",
			handlerFn: func() mcp.ResourceHandler {
				return RepositoryResourceContentsHandler(repositoryResourceContentURITemplate)
			},
			expectedResponseType: resourceResponseTypeText, // Ignored as error is expected
			expectError:          "owner is required",
		},
		{
			name: "missing repo",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetRawReposContentsByOwnerByRepoByBranchByPath: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "text/markdown")
					_, err := w.Write([]byte("# Test Repository\n\nThis is a test repository."))
					require.NoError(t, err)
				}),
			}),
			uri: "repo://owner//refs/heads/main/contents/README.md",
			handlerFn: func() mcp.ResourceHandler {
				return RepositoryResourceContentsHandler(repositoryResourceBranchContentURITemplate)
			},
			expectedResponseType: resourceResponseTypeText, // Ignored as error is expected
			expectError:          "repo is required",
		},
		{
			name: "successful blob content fetch",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetRawReposContentsByOwnerByRepoByPath: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "image/png")
					_, err := w.Write([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01"))
					require.NoError(t, err)
				}),
			}),
			uri: "repo://owner/repo/contents/data.png",
			handlerFn: func() mcp.ResourceHandler {
				return RepositoryResourceContentsHandler(repositoryResourceContentURITemplate)
			},
			expectedResponseType: resourceResponseTypeBlob,
			expectedResult: &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					Blob:     []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01"),
					MIMEType: "image/png",
					URI:      "",
				}}},
		},
		{
			name: "successful text content fetch (HEAD)",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetRawReposContentsByOwnerByRepoByPath: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "text/markdown")
					_, err := w.Write([]byte("# Test Repository\n\nThis is a test repository."))
					require.NoError(t, err)
				}),
			}),
			uri: "repo://owner/repo/contents/README.md",
			handlerFn: func() mcp.ResourceHandler {
				return RepositoryResourceContentsHandler(repositoryResourceContentURITemplate)
			},
			expectedResponseType: resourceResponseTypeText,
			expectedResult: &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					Text:     "# Test Repository\n\nThis is a test repository.",
					MIMEType: "text/markdown",
					URI:      "",
				}}},
		},
		{
			name: "successful text content fetch (HEAD)",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetRawReposContentsByOwnerByRepoByPath: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "text/plain")

					require.Contains(t, r.URL.Path, "pkg/github/actions.go")
					_, err := w.Write([]byte("package actions\n\nfunc main() {\n    // Sample Go file content\n}\n"))
					require.NoError(t, err)
				}),
			}),
			uri: "repo://owner/repo/contents/pkg/github/actions.go",
			handlerFn: func() mcp.ResourceHandler {
				return RepositoryResourceContentsHandler(repositoryResourceContentURITemplate)
			},
			expectedResponseType: resourceResponseTypeText,
			expectedResult: &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					Text:     "package actions\n\nfunc main() {\n    // Sample Go file content\n}\n",
					MIMEType: "text/plain",
					URI:      "",
				}}},
		},
		{
			name: "successful text content fetch (branch)",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetRawReposContentsByOwnerByRepoByBranchByPath: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "text/markdown")
					_, err := w.Write([]byte("# Test Repository\n\nThis is a test repository."))
					require.NoError(t, err)
				}),
			}),
			uri: "repo://owner/repo/refs/heads/main/contents/README.md",
			handlerFn: func() mcp.ResourceHandler {
				return RepositoryResourceContentsHandler(repositoryResourceBranchContentURITemplate)
			},
			expectedResponseType: resourceResponseTypeText,
			expectedResult: &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					Text:     "# Test Repository\n\nThis is a test repository.",
					MIMEType: "text/markdown",
					URI:      "",
				}}},
		},
		{
			name: "successful text content fetch (tag)",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetRawReposContentsByOwnerByRepoByTagByPath: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "text/markdown")
					_, err := w.Write([]byte("# Test Repository\n\nThis is a test repository."))
					require.NoError(t, err)
				}),
			}),
			uri: "repo://owner/repo/refs/tags/v1.0.0/contents/README.md",
			handlerFn: func() mcp.ResourceHandler {
				return RepositoryResourceContentsHandler(repositoryResourceTagContentURITemplate)
			},
			expectedResponseType: resourceResponseTypeText,
			expectedResult: &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					Text:     "# Test Repository\n\nThis is a test repository.",
					MIMEType: "text/markdown",
					URI:      "",
				}}},
		},
		{
			name: "successful text content fetch (sha)",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetRawReposContentsByOwnerByRepoBySHAByPath: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "text/markdown")
					_, err := w.Write([]byte("# Test Repository\n\nThis is a test repository."))
					require.NoError(t, err)
				}),
			}),
			uri: "repo://owner/repo/sha/abc123/contents/README.md",
			handlerFn: func() mcp.ResourceHandler {
				return RepositoryResourceContentsHandler(repositoryResourceCommitContentURITemplate)
			},
			expectedResponseType: resourceResponseTypeText,
			expectedResult: &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					Text:     "# Test Repository\n\nThis is a test repository.",
					MIMEType: "text/markdown",
					URI:      "",
				}}},
		},
		{
			name: "successful text content fetch (pr)",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetReposPullsByOwnerByRepoByPullNumber: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, err := w.Write([]byte(`{"head": {"sha": "abc123"}}`))
					require.NoError(t, err)
				}),
				GetRawReposContentsByOwnerByRepoBySHAByPath: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "text/markdown")
					_, err := w.Write([]byte("# Test Repository\n\nThis is a test repository."))
					require.NoError(t, err)
				}),
			}),
			uri: "repo://owner/repo/refs/pull/42/head/contents/README.md",
			handlerFn: func() mcp.ResourceHandler {
				return RepositoryResourceContentsHandler(repositoryResourcePrContentURITemplate)
			},
			expectedResponseType: resourceResponseTypeText,
			expectedResult: &mcp.ReadResourceResult{
				Contents: []*mcp.ResourceContents{{
					Text:     "# Test Repository\n\nThis is a test repository.",
					MIMEType: "text/markdown",
					URI:      "",
				}}},
		},
		{
			name: "content fetch fails",
			mockedClient: MockHTTPClientWithHandlers(map[string]http.HandlerFunc{
				GetReposContentsByOwnerByRepoByPath: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"message": "Not Found"}`))
				}),
			}),
			uri: "repo://owner/repo/contents/nonexistent.md",
			handlerFn: func() mcp.ResourceHandler {
				return RepositoryResourceContentsHandler(repositoryResourceContentURITemplate)
			},
			expectedResponseType: resourceResponseTypeText, // Ignored as error is expected
			expectError:          "404 Not Found",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client := mustNewGHClient(t, tc.mockedClient)
			mockRawClient, err := raw.NewClient(client, base)
			require.NoError(t, err)
			deps := BaseDeps{
				Client:    client,
				RawClient: mockRawClient,
			}
			ctx := ContextWithDeps(context.Background(), deps)
			handler := tc.handlerFn()

			request := &mcp.ReadResourceRequest{
				Params: &mcp.ReadResourceParams{
					URI: tc.uri,
				},
			}

			resp, err := handler(ctx, request)

			if tc.expectError != "" {
				require.ErrorContains(t, err, tc.expectError)
				return
			}

			require.NoError(t, err)

			content := resp.Contents[0]
			switch tc.expectedResponseType {
			case resourceResponseTypeBlob:
				require.Equal(t, tc.expectedResult.Contents[0].Blob, content.Blob)

				wireBytes, err := json.Marshal(resp)
				require.NoError(t, err)
				var wireResult struct {
					Contents []struct {
						Blob string `json:"blob"`
					} `json:"contents"`
				}
				require.NoError(t, json.Unmarshal(wireBytes, &wireResult))
				require.Len(t, wireResult.Contents, 1)
				decodedBlob, err := base64.StdEncoding.DecodeString(wireResult.Contents[0].Blob)
				require.NoError(t, err)
				require.Equal(t, tc.expectedResult.Contents[0].Blob, decodedBlob)
				require.True(t, strings.HasPrefix(string(decodedBlob), "\x89PNG\r\n\x1a\n"))
			case resourceResponseTypeText:
				require.Equal(t, tc.expectedResult.Contents[0].Text, content.Text)
			default:
				t.Fatalf("unknown expectedResponseType %v", tc.expectedResponseType)
			}
		})
	}
}

// Test_repositoryResourceContentsHandler_NetworkError tests that a network error
// during raw content fetch does not cause a panic (nil response body dereference).
func Test_repositoryResourceContentsHandler_NetworkError(t *testing.T) {
	base, _ := url.Parse("https://raw.example.com/")
	networkErr := errors.New("network error: connection refused")

	httpClient := &http.Client{Transport: &errorTransport{err: networkErr}}
	client := mustNewGHClient(t, httpClient)
	mockRawClient, err := raw.NewClient(client, base)
	require.NoError(t, err)
	deps := BaseDeps{
		Client:    client,
		RawClient: mockRawClient,
	}
	ctx := ContextWithDeps(context.Background(), deps)

	handler := RepositoryResourceContentsHandler(repositoryResourceContentURITemplate)

	request := &mcp.ReadResourceRequest{
		Params: &mcp.ReadResourceParams{
			URI: "repo://owner/repo/contents/README.md",
		},
	}

	// This should not panic, even though the HTTP client returns an error
	resp, err := handler(ctx, request)
	require.Error(t, err)
	require.Nil(t, resp)
	require.ErrorContains(t, err, "failed to get raw content")
}

func Test_expandRepoResourceURI(t *testing.T) {
	tests := []struct {
		name     string
		sha      string
		ref      string
		path     string
		expected string
	}{
		{
			name:     "ascii path on branch",
			ref:      "refs/heads/feature/x",
			path:     "docs/a b.md",
			expected: "repo://owner/repo/refs/heads/feature%2Fx/contents/docs/a%20b.md",
		},
		{
			name:     "non-ascii path",
			sha:      "abc123",
			path:     "testdata/ملف.txt",
			expected: "repo://owner/repo/sha/abc123/contents/testdata/%D9%85%D9%84%D9%81.txt",
		},
		{
			name:     "non-ascii tag",
			ref:      "refs/tags/v1-é",
			path:     "README.md",
			expected: "repo://owner/repo/refs/tags/v1-%C3%A9/contents/README.md",
		},
		{
			name:     "literal percent in path",
			path:     "patches/%D9.patch",
			expected: "repo://owner/repo/contents/patches/%25D9.patch",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			uri, err := expandRepoResourceURI("owner", "repo", tc.sha, tc.ref, strings.Split(tc.path, "/"))
			require.NoError(t, err)
			require.Equal(t, tc.expected, uri)

			parsed, err := url.Parse(uri)
			require.NoError(t, err)
			_, path, _ := strings.Cut(parsed.Path, "/contents/")
			require.Equal(t, tc.path, path)
		})
	}
}
