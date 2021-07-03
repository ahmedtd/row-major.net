// Package hackernews is a simple HTTP client for the Hacker News Firebase API.
package hackernews

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

// Client provides functions for interacting with the Hacker News API.
type Client struct {
	Client *http.Client
	Host   string
}

// New creates a new Client.
func New(client *http.Client, host string) *Client {
	return &Client{
		Client: client,
		Host:   host,
	}
}

const wantContentType = "application/json; charset=utf-8"

func (c *Client) doGet(ctx context.Context, url string, target interface{}) error {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("while making request: %w", err)
	}

	resp, err := c.Client.Do(req)
	if err != nil {
		return fmt.Errorf("while fetching %q: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code %d", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != wantContentType {
		return fmt.Errorf("bad Content-Type %q, want %q", resp.Header.Get("Content-Type"), wantContentType)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("while reading body: %w", err)
	}

	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("while unmarshaling body: %w", err)
	}

	return nil
}

// TopStories pulls the /v0/topstories endpoint.
func (c *Client) TopStories(ctx context.Context) ([]uint64, error) {
	url := &url.URL{
		Scheme: "https",
		Host:   c.Host,
		Path:   path.Join("v0", "topstories.json"),
	}

	topStories := []uint64{}
	if err := c.doGet(ctx, url.String(), &topStories); err != nil {
		return nil, fmt.Errorf("while getting top stories: %w", err)
	}
	return topStories, nil
}

// Item is a member of /v0/item.
type Item struct {
	ID          uint64   `json:"id"`
	Deleted     bool     `json:"deleted"`
	Type        string   `json:"type"`
	By          string   `json:"by"`
	Time        uint64   `json:"time"`
	Text        string   `json:"text"`
	Dead        bool     `json:"dead"`
	Parent      uint64   `json:"parent"`
	Poll        uint64   `json:"poll"`
	Kids        []uint64 `json:"kids"`
	URL         string   `json:"url"`
	Score       int64    `json:"score"`
	Title       string   `json:"title"`
	Parts       []uint64 `json:"parts"`
	Descendants uint64   `json:"descendants"`
}

// Item pulls a specific item from the /v0/item collection.
func (c *Client) Item(ctx context.Context, id uint64) (*Item, error) {
	tracer := otel.Tracer("row-major/rumor-mill/hackernews")
	var span trace.Span
	ctx, span = tracer.Start(ctx, "Client.Item")
	defer span.End()

	url := &url.URL{
		Scheme: "https",
		Host:   c.Host,
		Path:   path.Join("v0", "item", fmt.Sprintf("%v.json", id)),
	}

	item := &Item{}
	if err := c.doGet(ctx, url.String(), &item); err != nil {
		return nil, fmt.Errorf("while getting item %s: %w", id, err)
	}
	return item, nil
}

// Items pulls the specified items from the /v0/item collection.
func (c *Client) Items(ctx context.Context, ids []uint64) ([]*Item, error) {
	wg := &sync.WaitGroup{}
	var fetchErr error
	items := []*Item{}
	lock := &sync.Mutex{}

	for _, id := range ids {
		id := id
		wg.Add(1)
		go func() {
			item, err := c.Item(ctx, id)
			if err != nil {
				lock.Lock()
				fetchErr = fmt.Errorf("while retrieving item %d: %w", id, err)
				lock.Unlock()
			}
			lock.Lock()
			items = append(items, item)
			lock.Unlock()

			wg.Done()
		}()
	}
	wg.Wait()

	if fetchErr != nil {
		return nil, fetchErr
	}
	return items, nil
}
