package server

import (
	"net/http"
	"time"

	"github.com/getlantern/dhtup"
)

// An http.ResponseWriter that exposes the ability to instrument operations.
type InstrumentedResponseWriter interface {
	http.ResponseWriter
	// Set value for the given key in the current context
	Set(key string, value interface{})
	// Finish the current context
	Finish()
	// Fail the current context with the given error if error is not nil
	FailIf(err error)
}

// An implementation of InstrumentedResponseWriter that does nothing.
type NoopInstrumentedResponseWriter struct {
	http.ResponseWriter
}

func (rw *NoopInstrumentedResponseWriter) Set(key string, value interface{}) {
}

func (rw *NoopInstrumentedResponseWriter) Finish() {}

func (rw *NoopInstrumentedResponseWriter) FailIf(err error) {
}

// Set backup index values
// Leave durations as 0 for sane defaults
func (me *NewHttpHandlerInput) SetLocalIndex(
	res *dhtup.Resource,
	eventualFetchTimeout time.Duration,
	maxWaitDelayForPrimarySearchIndex time.Duration,
	requestInterceptor func(string, *http.Request) error) {
	me.LocalIndexDhtupResource = res
	me.MaxWaitDelayForPrimarySearchIndex = maxWaitDelayForPrimarySearchIndex
	if me.MaxWaitDelayForPrimarySearchIndex == 0 {
		me.MaxWaitDelayForPrimarySearchIndex = 3 * time.Second
	}
	me.DualSearchIndexRoundTripperInterceptRequestFunc = requestInterceptor
}

func (me *NewHttpHandlerInput) SetDefaults() {
	if me.HttpClient == nil {
		me.HttpClient = http.DefaultClient
	}
	if me.ProxiedRoundTripper == nil {
		me.ProxiedRoundTripper = http.DefaultTransport
	}
	if me.AddCommonHeaders == nil {
		me.AddCommonHeaders = func(r *http.Request) {}
	}
	if me.ProcessCORSHeaders == nil {
		me.ProcessCORSHeaders = func(responseHeaders http.Header, r *http.Request) bool {
			return false
		}
	}
	me.StoreMetainfoFileAndTokenLocally = true
	me.InstrumentResponseWriter = func(
		w http.ResponseWriter,
		label string,
	) InstrumentedResponseWriter {
		return &NoopInstrumentedResponseWriter{w}
	}

}
