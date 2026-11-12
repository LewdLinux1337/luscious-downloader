package netx

import (
	"context"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	HTTP    *http.Client
	Limiter *Limiter
}

type Limiter struct {
	permits chan struct{}
}

func New(timeout time.Duration, maxInFlight int, cookie string) *Client {
	if maxInFlight < 1 {
		maxInFlight = 1
	}
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          maxInFlight * 2,
		MaxIdleConnsPerHost:   maxInFlight,
		MaxConnsPerHost:       maxInFlight,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: time.Second,
		ForceAttemptHTTP2:     true,
	}
	var roundTripper http.RoundTripper = transport
	if cookie != "" {
		roundTripper = cookieTransport{base: transport, cookie: cookie}
	}
	return &Client{
		HTTP:    &http.Client{Transport: roundTripper, Timeout: timeout},
		Limiter: &Limiter{permits: make(chan struct{}, maxInFlight)},
	}
}

type cookieTransport struct {
	base   http.RoundTripper
	cookie string
}

func (t cookieTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	request := req.Clone(req.Context())
	request.Header = req.Header.Clone()
	host := strings.TrimSuffix(strings.ToLower(req.URL.Hostname()), ".")
	if strings.EqualFold(req.URL.Scheme, "https") && (host == "members.luscious.net" || host == "www.luscious.net") {
		request.Header.Set("Cookie", t.cookie)
	} else {
		request.Header.Del("Cookie")
	}
	return t.base.RoundTrip(request)
}

func (l *Limiter) Acquire(ctx context.Context) error {
	select {
	case l.permits <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *Limiter) Release() {
	<-l.permits
}
