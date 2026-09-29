package client

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
)

const (
	kvListsPath  = "/kv/rest/v1/lists"
	kvValuesPath = "/kv/rest/v1/values"
	kvPageSize   = 1000
)

// KVBatchLimit is the most items one list write may add and remove together.
// Larger changes are split into several writes, so a failure part way through
// leaves some of them applied.
const KVBatchLimit = 1500

// KVKeyPattern matches the list and value keys the Key-Value API accepts.
var KVKeyPattern = regexp.MustCompile(`^[A-Za-z0-9 ._:$-]+$`)

type kvEnvelope[T any] struct {
	Data   T      `json:"data"`
	Cursor string `json:"cursor"`
}

type kvListPage struct {
	Items []string `json:"items"`
}

type kvValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type kvListCreate struct {
	Key   string   `json:"key"`
	Items []string `json:"items"`
}

type kvListPatch struct {
	AddItems    []string `json:"addItems"`
	RemoveItems []string `json:"removeItems"`
}

func kvListPath(key string) string {
	return kvListsPath + "/" + url.PathEscape(key)
}

func kvValuePath(key string) string {
	return kvValuesPath + "/" + url.PathEscape(key)
}

// GetKVList returns every item in the list, in the API's sorted order. A list
// that does not exist reads the same as an empty one, so both return no items
// and no error.
func (c *Client) GetKVList(ctx context.Context, key string) ([]string, error) {
	var items []string
	cursor := ""
	for {
		query := url.Values{"limit": {strconv.Itoa(kvPageSize)}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var page kvEnvelope[kvListPage]
		if err := c.doJSON(ctx, "get key-value list", http.MethodGet, kvListPath(key)+"?"+query.Encode(), nil, &page); err != nil {
			return nil, err
		}
		items = append(items, page.Data.Items...)
		if page.Cursor == "" || len(page.Data.Items) == 0 {
			return items, nil
		}
		cursor = page.Cursor
	}
}

// CreateKVList writes a new list. Creating a list whose key already exists adds
// the items to it rather than replacing it, so callers check first.
func (c *Client) CreateKVList(ctx context.Context, key string, items []string) error {
	first := items
	if len(first) > KVBatchLimit {
		first = items[:KVBatchLimit]
	}
	if err := c.doJSON(ctx, "create key-value list", http.MethodPost, kvListsPath, kvListCreate{Key: key, Items: first}, nil); err != nil {
		return err
	}
	return c.UpdateKVList(ctx, key, items[len(first):], nil)
}

// UpdateKVList adds and removes items in batches of at most KVBatchLimit.
func (c *Client) UpdateKVList(ctx context.Context, key string, add, remove []string) error {
	for len(add) > 0 || len(remove) > 0 {
		removeCount := min(KVBatchLimit, len(remove))
		addCount := min(KVBatchLimit-removeCount, len(add))
		batch := kvListPatch{
			AddItems:    append([]string{}, add[:addCount]...),
			RemoveItems: append([]string{}, remove[:removeCount]...),
		}
		add, remove = add[addCount:], remove[removeCount:]
		if err := c.doJSON(ctx, "update key-value list", http.MethodPatch, kvListPath(key), batch, nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) DeleteKVList(ctx context.Context, key string) error {
	return c.doJSON(ctx, "delete key-value list", http.MethodDelete, kvListPath(key), nil, nil)
}

// GetKVValue returns an error that satisfies IsNotFound when the key has no
// value.
func (c *Client) GetKVValue(ctx context.Context, key string) (string, error) {
	var result kvEnvelope[kvValue]
	if err := c.doJSON(ctx, "get key-value value", http.MethodGet, kvValuePath(key), nil, &result); err != nil {
		return "", err
	}
	return result.Data.Value, nil
}

// SetKVValue stores the value, replacing any value the key already has.
func (c *Client) SetKVValue(ctx context.Context, key, value string) error {
	return c.doJSON(ctx, "set key-value value", http.MethodPost, kvValuesPath, kvValue{Key: key, Value: value}, nil)
}

func (c *Client) DeleteKVValue(ctx context.Context, key string) error {
	return c.doJSON(ctx, "delete key-value value", http.MethodDelete, kvValuePath(key), nil, nil)
}
