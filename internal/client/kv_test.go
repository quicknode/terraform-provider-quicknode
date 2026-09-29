package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
)

// fakeKVList stands in for one list on the Key-Value API. It enforces the
// batch limit, pages GET responses and records every request path.
type fakeKVList struct {
	items    []string
	pageSize int
	requests []string
}

func newFakeKVClient(t *testing.T, list *fakeKVList) *Client {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		list.requests = append(list.requests, r.Method+" "+r.URL.EscapedPath())
		w.Header().Set("Content-Type", "application/json")

		var body struct {
			Items       []string `json:"items"`
			AddItems    []string `json:"addItems"`
			RemoveItems []string `json:"removeItems"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		switch r.Method {
		case http.MethodPost, http.MethodPatch:
			if len(body.Items)+len(body.AddItems)+len(body.RemoveItems) > KVBatchLimit {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprint(w, `{"statusCode":400,"message":"too many items"}`)
				return
			}
			for _, item := range append(body.Items, body.AddItems...) {
				if !slices.Contains(list.items, item) {
					list.items = append(list.items, item)
				}
			}
			list.items = slices.DeleteFunc(list.items, func(item string) bool { return slices.Contains(body.RemoveItems, item) })
			slices.Sort(list.items)
			_, _ = fmt.Fprint(w, `{"code":200,"msg":"ok","data":null}`)
		default:
			start, _ := strconv.Atoi(r.URL.Query().Get("cursor"))
			end := min(start+list.pageSize, len(list.items))
			cursor := ""
			if end < len(list.items) {
				cursor = strconv.Itoa(end)
			}
			page, _ := json.Marshal(list.items[start:end])
			if len(list.items) == 0 {
				page = []byte("null")
			}
			_, _ = fmt.Fprintf(w, `{"code":200,"msg":"","data":{"items":%s},"cursor":%q}`, page, cursor)
		}
	}))
	t.Cleanup(server.Close)

	quicknode, err := New("test-key", WithBaseURL(server.URL), WithMaxRetries(0))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return quicknode
}

func numberedItems(prefix string, count int) []string {
	items := make([]string, count)
	for index := range items {
		items[index] = fmt.Sprintf("%s-%05d", prefix, index)
	}
	return items
}

func TestCreateKVListSplitsLargeListsIntoBatches(t *testing.T) {
	list := &fakeKVList{pageSize: 1000}
	quicknode := newFakeKVClient(t, list)

	if err := quicknode.CreateKVList(context.Background(), "wallets", numberedItems("item", 3200)); err != nil {
		t.Fatalf("CreateKVList: %v", err)
	}
	if len(list.items) != 3200 {
		t.Fatalf("list holds %d items, want 3200", len(list.items))
	}
	want := []string{"POST /kv/rest/v1/lists", "PATCH /kv/rest/v1/lists/wallets", "PATCH /kv/rest/v1/lists/wallets"}
	if !slices.Equal(list.requests, want) {
		t.Fatalf("requests = %v, want %v", list.requests, want)
	}
}

func TestUpdateKVListSharesTheBatchLimitBetweenAddsAndRemoves(t *testing.T) {
	list := &fakeKVList{pageSize: 1000, items: numberedItems("old", 1000)}
	quicknode := newFakeKVClient(t, list)

	if err := quicknode.UpdateKVList(context.Background(), "wallets", numberedItems("new", 1000), numberedItems("old", 1000)); err != nil {
		t.Fatalf("UpdateKVList: %v", err)
	}
	if !slices.Equal(list.items, numberedItems("new", 1000)) {
		t.Fatalf("list holds %d items starting %q, want the 1000 new ones", len(list.items), list.items[0])
	}
	if len(list.requests) != 2 {
		t.Fatalf("made %d requests, want 2: %v", len(list.requests), list.requests)
	}
}

func TestGetKVListFollowsTheCursor(t *testing.T) {
	list := &fakeKVList{pageSize: 2, items: []string{"a", "b", "c", "d", "e"}}
	quicknode := newFakeKVClient(t, list)

	items, err := quicknode.GetKVList(context.Background(), "wallets")
	if err != nil {
		t.Fatalf("GetKVList: %v", err)
	}
	if !slices.Equal(items, list.items) {
		t.Fatalf("items = %v, want %v", items, list.items)
	}
	if len(list.requests) != 3 {
		t.Fatalf("made %d requests, want 3", len(list.requests))
	}
}

func TestGetKVListReadsAMissingListAsEmpty(t *testing.T) {
	quicknode := newFakeKVClient(t, &fakeKVList{pageSize: 1000})

	items, err := quicknode.GetKVList(context.Background(), "missing")
	if err != nil {
		t.Fatalf("GetKVList: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %v, want none", items)
	}
}

func TestGetKVValueReportsAMissingKeyAsNotFound(t *testing.T) {
	quicknode, recorded := newRecordingClient(t, http.StatusNotFound, `{"statusCode":404,"message":"Key not found"}`)

	_, err := quicknode.GetKVValue(context.Background(), "threshold")
	if !IsNotFound(err) {
		t.Fatalf("err = %v, want not found", err)
	}
	if recorded.path != "/kv/rest/v1/values/threshold" {
		t.Fatalf("path = %q", recorded.path)
	}
}

func TestSetKVValueSendsKeyAndValue(t *testing.T) {
	quicknode, recorded := newRecordingClient(t, http.StatusCreated, `{"code":200,"msg":"Key value stored","data":null}`)

	if err := quicknode.SetKVValue(context.Background(), "threshold", "42"); err != nil {
		t.Fatalf("SetKVValue: %v", err)
	}
	if recorded.method != http.MethodPost || recorded.body["key"] != "threshold" || recorded.body["value"] != "42" {
		t.Fatalf("request = %s %v", recorded.method, recorded.body)
	}
}

func TestEveryRequestCarriesTheUserAgent(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("User-Agent"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":200,"msg":"","data":{"key":"threshold","value":"1"}}`)
	}))
	t.Cleanup(server.Close)

	quicknode, err := New("test-key", WithBaseURL(server.URL), WithUserAgent("quicknode-terraform/0.3.0 (linux-x86_64; terraform-1.9.8)"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := quicknode.GetKVValue(context.Background(), "threshold"); err != nil {
		t.Fatalf("GetKVValue: %v", err)
	}
	_, _ = quicknode.ListChains(context.Background())
	for _, userAgent := range seen {
		if userAgent != "quicknode-terraform/0.3.0 (linux-x86_64; terraform-1.9.8)" {
			t.Fatalf("User-Agent = %q", userAgent)
		}
	}
	if len(seen) != 2 {
		t.Fatalf("server saw %d requests, want 2", len(seen))
	}
}
