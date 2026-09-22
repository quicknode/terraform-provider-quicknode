package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// rateLimitsBody mixes the two sources the route reports. Only a user_override
// carries an id, which is what makes it deletable.
const rateLimitsBody = `{"data":{"rate_limits":[
{"bucket":"rps","rate_limit":25,"source":"plan_default"},
{"bucket":"rpm","rate_limit":500,"source":"plan_default"},
{"bucket":"rpd","rate_limit":-1,"source":"plan_default"},
{"bucket":"rps","rate_limit":10,"source":"user_override","id":"ovr-1"}]}}`

func TestGetRateLimitsKeepsSourceAndID(t *testing.T) {
	limits, err := newTestClient(t, rateLimitsBody).GetRateLimits(context.Background(), "1")
	if err != nil {
		t.Fatalf("GetRateLimits: %v", err)
	}
	if len(limits) != 4 {
		t.Fatalf("got %d limits, want 4", len(limits))
	}

	var override *RateLimit
	for index, limit := range limits {
		if limit.Source == SourceUserOverride {
			override = &limits[index]
		}
	}
	if override == nil {
		t.Fatal("the user override was lost")
	}
	if override.Bucket != BucketRPS || override.Value != 10 || override.ID != "ovr-1" {
		t.Errorf("override = %+v", *override)
	}

	for _, limit := range limits {
		if limit.Source == SourcePlanDefault && limit.ID != "" {
			t.Errorf("a plan default carried an id: %+v", limit)
		}
	}
	if limits[2].Value != RateLimitUnset {
		t.Errorf("an unlimited bucket should stay at %d, got %d", RateLimitUnset, limits[2].Value)
	}
}

// TestSetRateLimitsOmitsUnmanagedBuckets guards the same hazard the security
// toggles have: a bucket the configuration does not set must stay out of the
// body rather than being sent as a zero.
func TestSetRateLimitsOmitsUnmanagedBuckets(t *testing.T) {
	var captured map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"limits":{"rps":10,"rpm":-1,"rpd":-1}}}`))
	}))
	t.Cleanup(server.Close)

	quicknode, err := New("test-key", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rps := 10
	if err := quicknode.SetRateLimits(context.Background(), "1", RateLimitOverrides{RPS: &rps}); err != nil {
		t.Fatalf("SetRateLimits: %v", err)
	}

	limits, ok := captured["rate_limits"].(map[string]any)
	if !ok {
		t.Fatalf("request carried no rate_limits object: %v", captured)
	}
	if limits[BucketRPS] != float64(10) {
		t.Errorf("rps = %v, want 10", limits[BucketRPS])
	}
	for _, unmanaged := range []string{BucketRPM, BucketRPD} {
		if _, present := limits[unmanaged]; present {
			t.Errorf("%s was sent although the caller left it unset", unmanaged)
		}
	}
}

func TestDeleteRateLimitOverrideRejectsFalseSuccess(t *testing.T) {
	quicknode := newTestClient(t, `{"data":{"success":false}}`)

	if err := quicknode.DeleteRateLimitOverride(context.Background(), "1", "ovr-1"); err == nil {
		t.Fatal("a success:false delete has to be an error, not a silent no-op")
	}
}

func TestRateLimitDeletesTolerateAbsence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	quicknode, err := New("test-key", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx := context.Background()
	if err := quicknode.DeleteRateLimitOverride(ctx, "1", "ovr-1"); err != nil {
		t.Errorf("removing an absent override should succeed, got %v", err)
	}
	if err := quicknode.RemoveMethodRateLimit(ctx, "1", "lim-1"); err != nil {
		t.Errorf("removing an absent method rate limit should succeed, got %v", err)
	}
}

func TestListMethodRateLimits(t *testing.T) {
	const body = `{"data":{"rate_limiters":[{"id":"lim-1","interval":"second","rate":5,
"status":"enabled","created":"1742400000","methods":["eth_getLogs","eth_call"]}]}}`

	limiters, err := newTestClient(t, body).ListMethodRateLimits(context.Background(), "1")
	if err != nil {
		t.Fatalf("ListMethodRateLimits: %v", err)
	}
	if len(limiters) != 1 {
		t.Fatalf("got %d limiters, want 1", len(limiters))
	}
	limiter := limiters[0]
	if limiter.ID != "lim-1" || limiter.Rate != 5 || limiter.Interval != "second" || limiter.Status != "enabled" {
		t.Errorf("limiter = %+v", limiter)
	}
	if len(limiter.Methods) != 2 || limiter.Methods[0] != "eth_getLogs" {
		t.Errorf("Methods = %v", limiter.Methods)
	}
}

// TestUpdateMethodRateLimitSendsWholeObject records that the update route
// replaces the limiter rather than merging a delta, and that it takes no
// interval.
func TestUpdateMethodRateLimitSendsWholeObject(t *testing.T) {
	var captured map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"lim-1","rate":9,"status":"disabled","methods":["eth_call"]}}`))
	}))
	t.Cleanup(server.Close)

	quicknode, err := New("test-key", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = quicknode.UpdateMethodRateLimit(context.Background(), "1", "lim-1", MethodRateLimit{
		Methods: []string{"eth_call"},
		Rate:    9,
		Status:  "disabled",
	})
	if err != nil {
		t.Fatalf("UpdateMethodRateLimit: %v", err)
	}
	if captured["rate"] != float64(9) || captured["status"] != "disabled" {
		t.Errorf("body = %v", captured)
	}
	if _, present := captured["interval"]; present {
		t.Error("the update route does not accept an interval, so one must not be sent")
	}
}
