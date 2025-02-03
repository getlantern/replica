package server

import (
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

type proxyTransport struct {
	// Satisfies http.RoundTripper
	client *http.Client
}

func (pt *proxyTransport) RoundTrip(req *http.Request) (resp *http.Response, err error) {
	if req.Method == "OPTIONS" {
		// No need to proxy the OPTIONS request.
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Connection":                   {"keep-alive"},
				"Access-Control-Allow-Methods": {"GET"},
				"Access-Control-Allow-Headers": {req.Header.Get("Access-Control-Request-Headers")},
				"Via":                          {"Lantern Client"},
			},
			Body: ioutil.NopCloser(strings.NewReader("preflight complete")),
		}, nil
	}

	req.Header.Del("Origin")
	resp, err = pt.client.Do(req)
	if err != nil {
		log.Errorf("Could not issue HTTP request: %v", err)
		return
	}
	return
}

func prepareRequest(r *http.Request, input NewHttpHandlerInput, serviceUrl func() *url.URL) {
	// The Iran region endpoint tries to redirect to https, and our proxy handler doesn't follow
	// redirects. I'm not sure why we were clobbering to "http" before. We need to make sure this
	// works regardless of region.
	log.Debugf("request url: %#v", r.URL)
	log.Debugf("request url path: %q", r.URL.Path)

	r.URL = serviceUrl().ResolveReference(&url.URL{
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
	})
	log.Debugf("final request url: %q", r.URL)
	r.RequestURI = "" // http: Request.RequestURI can't be set in client requests.

	input.AddCommonHeaders(r)
}

func proxyHandler(input NewHttpHandlerInput, modifyResponse func(*http.Response) error) http.Handler {
	return &httputil.ReverseProxy{
		Transport: &proxyTransport{
			client: &http.Client{
				Transport: input.HttpClient.Transport,
				// Don't follow redirects
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			},
		},
		Director: func(r *http.Request) {
			prepareRequest(r, input, input.ReplicaServiceClient.ReplicaServiceEndpoint)
		},
		ModifyResponse: modifyResponse,
	}
}
