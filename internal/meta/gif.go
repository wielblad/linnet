package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"go.mau.fi/mautrix-meta/pkg/messagix/graphql"
)

// The search Messenger's own GIF window asks. The library the program stands on does not
// know this question; its name, its number and what it is asked with were read from
// Messenger's page (26 November 2026). Meta changes such numbers now and then.
const gifQuery = "CometAnimatedImagePickerSearchResultsRootQuery"

func init() {
	graphql.GraphQLDocs[gifQuery] = graphql.GraphQLDoc{DocID: "23910503388569859", FriendlyName: gifQuery}
}

// GIF is one result of a search.
type GIF struct {
	ID            string
	Title         string
	URL           string // the GIF itself
	Width, Height int
	Bytes         int64
	Source        string // the service it comes from, such as GIPHY
}

type gifResponse struct {
	Data struct {
		Media struct {
			Groups []struct {
				Results []struct {
					ID     string `json:"result_id"`
					Source struct {
						Mime   string `json:"mime_type"`
						URL    string `json:"url"`
						Width  int    `json:"width"`
						Height int    `json:"height"`
						Bytes  int64  `json:"size_bytes"`
						Title  string `json:"title"`
						From   string `json:"gif_source"`
					} `json:"source"`
				} `json:"results"`
			} `json:"result_group"`
		} `json:"comet_external_media"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// SearchGIFs asks Messenger for GIFs: those for the words given, or the ones in fashion
// when there are none. The words go to Meta with the account's name on them, as they do
// from Messenger's own window in a chat that is not encrypted.
func (c *Client) SearchGIFs(ctx context.Context, words string) ([]GIF, error) {
	if err := c.waitReady(ctx); err != nil {
		return nil, err
	}
	words = strings.TrimSpace(words)
	kind := "SEARCH"
	if words == "" {
		kind = "TRENDING"
	}
	_, body, err := c.mx.GetHTTP().MakeGraphQLRequest(ctx, gifQuery, map[string]any{
		"media_height":     200,
		"media_width":      200,
		"query_type":       kind,
		"result_types":     []string{"ANIMATION"},
		"scale":            1,
		"search_interface": "MESSENGER_INTERFACE",
		"search_query":     words,
	})
	if err != nil {
		return nil, fmt.Errorf("search gifs: %w", err)
	}
	return ParseGIFs(body)
}

// ParseGIFs reads the answer to a search.
func ParseGIFs(body []byte) ([]GIF, error) {
	if i := strings.IndexByte(string(body), '\n'); i > 0 { // more than one answer may follow the first
		body = body[:i]
	}
	var resp gifResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("search gifs: answer not understood: %w", err)
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("search gifs: %s", resp.Errors[0].Message)
	}
	var out []GIF
	seen := map[string]bool{}
	for _, group := range resp.Data.Media.Groups {
		for i, r := range group.Results {
			s := r.Source
			if !strings.HasPrefix(s.URL, "https://") || s.Width <= 0 || s.Height <= 0 || (s.Mime != "" && s.Mime != "image/gif") {
				continue
			}
			id := r.ID
			if id == "" {
				id = "n" + strconv.Itoa(i)
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, GIF{ID: id, Title: s.Title, URL: s.URL, Width: s.Width, Height: s.Height, Bytes: s.Bytes, Source: s.From})
		}
	}
	return out, nil
}
