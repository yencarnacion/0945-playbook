package data

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

type rewriteTransport struct{ target *url.URL }

func (t rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	c := r.Clone(r.Context())
	c.URL.Scheme = t.target.Scheme
	c.URL.Host = t.target.Host
	return http.DefaultTransport.RoundTrip(c)
}
func testMassiveProvider(t *testing.T, h http.Handler) *MassiveProvider {
	t.Helper()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Skipf("loopback unavailable: %v", err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Listener = listener
	srv.Start()
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	hc := &http.Client{Timeout: time.Second, Transport: metaTransport{base: rewriteTransport{target: u}}}
	return newMassiveProviderWithHTTP("key", 1, "minute", true, time.UTC, hc)
}
func TestMassiveProviderNormalizesHTTPStatuses(t *testing.T) {
	for _, status := range []int{429, 500, 502, 503, 504, 401, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			p := testMassiveProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Retry-After", "3")
				w.Header().Set("X-Request-ID", "req-1")
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"status":"ERROR","error":"provider failure","request_id":"body-id"}`))
			}))
			_, err := p.FetchBars(context.Background(), "X", time.Now().Add(-time.Minute), time.Now())
			var he *HTTPError
			if !errors.As(err, &he) {
				t.Fatalf("error=%T %v", err, err)
			}
			wantRetry := status == 429 || status >= 500
			if he.Status != status || he.Retryable != wantRetry || he.RetryAfter != "3" || he.RequestID != "req-1" {
				t.Fatalf("normalized=%+v", he)
			}
		})
	}
}
func TestMassiveProviderGovernorRecoversAfterReal503(t *testing.T) {
	var calls atomic.Int32
	p := testMassiveProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(503)
			_, _ = w.Write([]byte(`{"status":"ERROR","error":"busy"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"OK","results":[{"t":1782999000000,"o":10,"h":11,"l":9,"c":10.5,"v":100,"vw":10.2}]}`))
	}))
	g := NewGovernor(100, 2, 3)
	defer g.Close()
	gp := NewGovernedProvider(p, g)
	bars, err := gp.FetchBars(context.Background(), "X", time.Now().Add(-time.Minute), time.Now())
	if err != nil || len(bars) != 1 || calls.Load() != 3 {
		t.Fatalf("bars=%d calls=%d err=%v", len(bars), calls.Load(), err)
	}
	m := g.Metrics()
	if m.Retries != 2 || m.Requests != 3 {
		t.Fatalf("metrics=%+v", m)
	}
}

func TestRetryAfterHTTPDateAndSeconds(t *testing.T) {
	now := time.Date(2026, 7, 20, 13, 30, 0, 0, time.UTC)
	if d, ok := retryAfterDelay("3", now); !ok || d != 3*time.Second {
		t.Fatalf("seconds delay=%s ok=%v", d, ok)
	}
	date := now.Add(5 * time.Second).Format(http.TimeFormat)
	if d, ok := retryAfterDelay(date, now); !ok || d != 5*time.Second {
		t.Fatalf("date delay=%s ok=%v", d, ok)
	}
}

func TestMassiveProviderMalformedResponseIsNotRetried(t *testing.T) {
	p := testMassiveProvider(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[`))
	}))
	_, err := p.FetchBars(context.Background(), "X", time.Now().Add(-time.Minute), time.Now())
	var he *HTTPError
	if !errors.As(err, &he) || he.Retryable {
		t.Fatalf("malformed response classification: %T %+v", err, he)
	}
}
