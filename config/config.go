package replicaConfig

import (
	"net/url"

	"github.com/getlantern/golog"
	"github.com/mitchellh/mapstructure"

	replicaServer "github.com/getlantern/replica/server"
	replicaService "github.com/getlantern/replica/service"
)

var log = golog.LoggerFor("replica.config")

// mimics the same interface from Flashlight
type FeatureOptions interface {
	FromMap(map[string]interface{}) error
}

type ReplicaOptionsGetter func() replicaServer.ReplicaOptions

// This returns a function that extracts the ReplicaOptions for the country determined by
// getCountry. The config is extracted via flashlight's feature options. Logging and geolookup
// refreshing is done consistently as per android-lantern and lantern-desktop. The function does not
// fail, it will always return a reasonable fallback option.
func NewReplicaOptionsGetter(
	populateReplicaOptions func(FeatureOptions) error,
	getCountry func() (string, error),
	refreshGeolocation func(),
) ReplicaOptionsGetter {
	return func() replicaServer.ReplicaOptions {
		var root ReplicaOptionsRoot
		if err := populateReplicaOptions(&root); err != nil {
			log.Errorf(
				"Could not fetch replica feature options: %v",
				err,
			)
			refreshGeolocation()
			return replicaServer.FallbackReplicaOptions{}
		}
		countryCode, err := getCountry()
		if err != nil {
			log.Errorf("while selecting replica options: error getting country: %v", err)
			log.Debugf("refreshing geolocation")
			refreshGeolocation()
		}
		opts, ok := root.ByCountry[countryCode]
		if ok {
			log.Debugf(
				"selected replica options for country %q",
				countryCode,
			)
		} else {
			log.Debugf("no country-specific replica options for %q. using default", countryCode)
			opts = root.ReplicaOptions
		}
		return &opts
	}
}

// This extracts a URL from the Replica Options providing fallback and logging for bad
// configuration.
func GetReplicaServiceEndpointUrl(opts replicaServer.ReplicaOptions) *url.URL {
	endpointStr := opts.GetReplicaRustEndpoint()
	url, err := url.Parse(endpointStr)
	if err != nil {
		log.Errorf(
			"parsing replica rust endpoint %q: %v",
			endpointStr,
			err,
		)
		// I'd rather pull from FallbackReplicaOptions.GetReplicaRustEndpoint, but that returns a
		// string. If we failed to parse that, we're in panic territory since this function can't
		// fail.
		url = replicaService.GlobalChinaDefaultServiceUrl
	}
	log.Debugf("using replica rust endpoint %q", url.String())
	return url
}

type ReplicaOptionsRoot struct {
	// This is the default.
	ReplicaOptions `mapstructure:",squash"`
	// Options tailored to country. This could be used to pattern match any arbitrary string really.
	// mapstructure should ignore the field name.
	ByCountry map[string]ReplicaOptions `mapstructure:",remain"`
	// Deprecated. An unmatched country uses the embedded ReplicaOptions.ReplicaRustEndpoint.
	// Removing this will break unmarshalling config.
	ReplicaRustDefaultEndpoint string
	// Deprecated. Use ByCountry.ReplicaRustEndpoint.
	ReplicaRustEndpoints map[string]string
}

// Implements the interface FeatureOptions from flashlight
func (ro *ReplicaOptionsRoot) FromMap(m map[string]interface{}) error {
	return mapstructure.Decode(m, ro)
}

type ReplicaOptions struct {
	// Use infohash and old-style prefixing simultaneously for now. Later, the old-style can be removed.
	WebseedBaseUrls []string
	Trackers        []string
	StaticPeerAddrs []string
	// Merged with the webseed URLs when the metadata and data buckets are merged.
	MetadataBaseUrls []string
	// The replica-rust endpoint to use. There's only one because object uploads and ownership are
	// fixed to a specific bucket, and replica-rust endpoints are 1:1 with a bucket.
	ReplicaRustEndpoint string
	// A set of info hashes (20 bytes, hex-encoded) to which proxies should announce themselves.
	ProxyAnnounceTargets []string
	// A set of info hashes where p2p-proxy peers can be found.
	ProxyPeerInfoHashes []string
	CustomCA            string
}

func (ro *ReplicaOptions) GetWebseedBaseUrls() []string {
	return ro.WebseedBaseUrls
}

func (ro *ReplicaOptions) GetTrackers() []string {
	return ro.Trackers
}

func (ro *ReplicaOptions) GetStaticPeerAddrs() []string {
	return ro.StaticPeerAddrs
}

func (ro *ReplicaOptions) GetMetadataBaseUrls() []string {
	return ro.MetadataBaseUrls
}

func (ro *ReplicaOptions) GetReplicaRustEndpoint() string {
	return ro.ReplicaRustEndpoint
}

func (ro *ReplicaOptions) GetCustomCA() string {
	return ro.CustomCA
}

// XXX <11-07-2022, soltzen> DEPREACTED in favor of
// github.com/getlantern/libp2p
func (ro *ReplicaOptions) GetProxyAnnounceTargets() []string {
	return nil
}
