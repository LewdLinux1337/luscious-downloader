package netx

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCookieTransportScopesCookieToLusciousHTTPS(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{url: "https://members.luscious.net/graphql", want: "session=secret"},
		{url: "https://www.luscious.net/pictures/example.jpg", want: "session=secret"},
		{url: "https://cdn.luscious.net/pictures/example.jpg", want: ""},
		{url: "http://www.luscious.net/pictures/example.jpg", want: ""},
		{url: "https://example.com/", want: ""},
	}
	for _, test := range tests {
		var got string
		transport := cookieTransport{
			cookie: "session=secret",
			base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				got = req.Header.Get("Cookie")
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
			}),
		}
		req, err := http.NewRequest(http.MethodGet, test.url, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Cookie", "caller=must-not-leak")
		response, err := transport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if got != test.want {
			t.Errorf("Cookie for %s = %q, want %q", test.url, got, test.want)
		}
	}
}
