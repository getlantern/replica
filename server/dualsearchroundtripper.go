package server

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"time"
)

const PrimarySearchRoundTripperKey = "primary"
const LocalIndexRoundTripperKey = "local_index"
const roundTripperHeaderKey = "RoundTripper"
const maxWaitDelayForPrimarySearchIndex = time.Second * 5

type DualSearchIndexRoundTripper struct {
	input NewHttpHandlerInput
}

func runSearchRoundTripper(
	req *http.Request,
	rt http.RoundTripper,
	rtKey string,
	ch chan<- *http.Response,
	interceptorFn func(string, *http.Request) error,
) {
	if interceptorFn != nil {
		if err := interceptorFn(rtKey, req); err != nil {
			close(ch)
			return
		}
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		// XXX <10-03-22, soltzen> Error here is ignorable since the
		// other roundtripper might still save the day
		log.Debugf("ignorable error while running search roundtripper [%s] with req [%s]: %v",
			rtKey, req.URL.String(), err)
		close(ch)
		return
	}
	if resp == nil {
		log.Debugf("ignorable error while running search roundtripper [%s] with req [%s]: response is nil",
			rtKey, req.URL.String(), err)
		close(ch)
		return
	}
	// Accept 1xx, 2xx and 3xx responses only
	if resp.StatusCode/100 >= 4 {
		defer close(ch)
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}
		resp.Body.Close()
		log.Debugf(
			"ignorable error while running search roundtripper [%s] with req [%s]: response has bad status code [%v] with body [%v]",
			rtKey, req.URL.String(), resp.StatusCode, string(b))
		return
	}
	// Add a header for the used roundtripper, mostly for testing
	resp.Header.Set(roundTripperHeaderKey, rtKey)
	ch <- resp
}

func copyRequest(req *http.Request) (*http.Request, *http.Request, error) {
	req2 := req.Clone(req.Context())
	if req.Body != nil {
		b, err := ioutil.ReadAll(req.Body)
		if err != nil {
			return nil, nil, log.Errorf("while reading request body %v", err)
		}
		req.Body = ioutil.NopCloser(bytes.NewReader(b))
		req2.Body = ioutil.NopCloser(bytes.NewReader(b))
	}
	return req, req2, nil
}

// RoundTrip() will run a search request through the primary roundtripper (a
// remote replica-rust instance) and through a local index roundtripper (a local
// sqlite search index), if that one exists, while prefering the primary
// roundtripper.
//
// This means that, even if the local index roundtripper fetched a value first,
// RoundTrip() will wait a bit (namely, half the deadline length of the
// context, or a constant short amount) before it returns the local index
// roundtripper's response.
func (a *DualSearchIndexRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// If there's no local index index, just run it usually
	if a.input.LocalIndexDhtDownloader == nil {
		return a.input.HttpClient.Transport.RoundTrip(req)
	}

	// Run the primary and local index roundtrippers in parallel
	req, req2, err := copyRequest(req)
	if err != nil {
		return nil, err
	}
	primaryRespChan := make(chan *http.Response)
	backupRespChan := make(chan *http.Response)
	go runSearchRoundTripper(req,
		a.input.HttpClient.Transport,
		PrimarySearchRoundTripperKey,
		primaryRespChan,
		a.input.DualSearchIndexRoundTripperInterceptRequestFunc,
	)
	go runSearchRoundTripper(req2,
		&LocalIndexRoundTripper{a.input.LocalIndexDhtDownloader},
		LocalIndexRoundTripperKey,
		backupRespChan,
		a.input.DualSearchIndexRoundTripperInterceptRequestFunc,
	)

	// Half the deadline length of the context: we'll wait this much on the
	// primary search index before using the local index
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	var primarySearchIndexDeadline time.Duration
	t, ok := ctx.Deadline()
	if !ok {
		primarySearchIndexDeadline = maxWaitDelayForPrimarySearchIndex
	} else {
		// Half the deadline length
		a := t.Sub(time.Now()) / 2
		if a == 0 {
			primarySearchIndexDeadline = maxWaitDelayForPrimarySearchIndex
		} else {
			primarySearchIndexDeadline = a
		}
	}

	// Wait for the primary search index to return a response,
	// Or for a context timeout
	// Or for the primary search index deadline to timeout
	select {
	case resp := <-primaryRespChan:
		if resp != nil {
			return resp, nil
		} else {
			log.Debugf("Received nil response from primary search roundtripper. Checking local index roundtripper...")
		}
	case <-time.After(primarySearchIndexDeadline):
		log.Debugf("Primary search roundtripper timed out. Checking local index roundtripper...")
	case <-ctx.Done():
		return nil, log.Errorf("Failed to make a search request with any roundtripper: %v", ctx.Err())
	}

	// If we reached here, the primary search index failed to return a response
	// and we're trying our luck with the local index before the context times
	// out
	select {
	case resp := <-backupRespChan:
		if resp != nil {
			return resp, nil
		} else {
			return nil, log.Errorf("Received nil response from local index search roundtripper. This is very weird.")
		}
	case <-ctx.Done():
		return nil, log.Errorf("Failed to make a search request with any roundtripper: %v", ctx.Err())
	}
}
