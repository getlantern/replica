package replica

import (
	"errors"
	"net/url"
	"sync"
)

var ERR_DYNAMICENDPOINT__URL_PARSE = errors.New("ERR_DYNAMICENDPOINT__URL_PARSE")

type DynamicEndpoint struct {
	val        *url.URL
	updateChan chan bool
	mutex      sync.RWMutex
}

func (self *DynamicEndpoint) set(newVal *url.URL) {
	self.mutex.Lock()
	defer self.mutex.Unlock()
	self.val = newVal
}

func (self *DynamicEndpoint) Get() *url.URL {
	self.mutex.RLock()
	defer self.mutex.RUnlock()
	return self.val
}

func NewDynamicEndpoint(
	defaultEndpoint string,
	conditionChan <-chan bool,
	errChan chan error,
	announceUpdates bool,
	newEndpointGetter func() (string, error),
) (*DynamicEndpoint, error) {
	defaultEndpointUrl, err := url.Parse(defaultEndpoint)
	if err != nil {
		return nil, err
	}
	dynamicEndpoint := &DynamicEndpoint{
		val: defaultEndpointUrl,
	}
	if announceUpdates {
		dynamicEndpoint.updateChan = make(chan bool, 100)
	}
	if newEndpointGetter != nil {
		go func() {
			for {
				<-conditionChan
				newEndpoint, err := newEndpointGetter()
				if err != nil {
					if errChan != nil {
						errChan <- err
					}
					continue
				}
				newUrl, err := url.Parse(newEndpoint)
				if err != nil {
					if errChan != nil {
						errChan <- err
					}
					continue
				}
				// Set the new value
				dynamicEndpoint.set(newUrl)
				// Send a signal on the update channel, if not nil
				if dynamicEndpoint.updateChan != nil {
					dynamicEndpoint.updateChan <- true
				}
			}
		}()
	}
	return dynamicEndpoint, nil
}

func (self *DynamicEndpoint) OnUpdate() <-chan bool {
	return self.updateChan
}

func NewConstantDynamicEndpoint(endpoint string) (*DynamicEndpoint, error) {
	return NewDynamicEndpoint(endpoint, nil, nil, false, nil)
}
