package server

import (
	"context"
	"io/ioutil"
	"net/http"
	"time"
)

const PrimarySearchRoundTripperKey = "primary"
const LocalIndexRoundTripperKey = "local_index"
const roundTripperHeaderKey = "RoundTripper"

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
	if resp.StatusCode != http.StatusOK {
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

// RoundTrip() will run a search request through the primary roundtripper (a
// remote replica-rust instance) and through a local index roundtripper (a local
// sqlite search index), if that one exists, while prefering the primary
// roundtripper.
//
// This means that, even if the local index roundtripper fetched a value first,
// RoundTrip() will wait a bit (namely, input.MaxWaitDelayForPrimarySearchIndex
// length of time) before it returns the local index roundtripper's response.
func (a *DualSearchIndexRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// If there's no local index index, just run it usually
	if a.input.LocalIndexDhtupResource == nil {
		return a.input.ProxiedRoundTripper.RoundTrip(req)
	}

	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	primaryRespChan := make(chan *http.Response)
	backupRespChan := make(chan *http.Response)
	go runSearchRoundTripper(req,
		a.input.ProxiedRoundTripper,
		PrimarySearchRoundTripperKey,
		primaryRespChan,
		a.input.DualSearchIndexRoundTripperInterceptRequestFunc,
	)
	go runSearchRoundTripper(req,
		&LocalIndexIndexRoundTripper{a.input, ctx},
		LocalIndexRoundTripperKey,
		backupRespChan,
		a.input.DualSearchIndexRoundTripperInterceptRequestFunc,
	)

	select {
	case resp := <-primaryRespChan:
		if resp != nil {
			return resp, nil
		} else {
			log.Debugf("Received nil response from primary search roundtripper. Checking local index roundtripper...")
		}
	case <-time.After(a.input.MaxWaitDelayForPrimarySearchIndex):
		// TODO <15-03-22, soltzen> Maybe make this wait duration halfway through the context's timeout?
		log.Debugf("Primary search roundtripper timedout. checking local index roundtripper...")
	case <-ctx.Done():
		log.Debugf("Primary search roundtripper timedout. checking local index roundtripper...")
	}

	select {
	case resp := <-backupRespChan:
		if resp != nil {
			return resp, nil
		}
	case <-ctx.Done():
	}
	return nil, log.Errorf("Failed to make a search request with any roundtripper: %v", ctx.Err())
}
