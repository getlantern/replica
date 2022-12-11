package replicaConfig

import (
	"net/url"

	"github.com/getlantern/flashlight"
	"github.com/getlantern/flashlight/config"
	"github.com/getlantern/flashlight/geolookup"
	"github.com/getlantern/golog"
	replicaServer "github.com/getlantern/replica/server"
	replicaService "github.com/getlantern/replica/service"
)

var log = golog.LoggerFor("replica.config")

type ReplicaOptionsGetter func() replicaServer.ReplicaOptions

// This returns a function that extracts the ReplicaOptions for the country determined by
// getCountry. The config is extracted via flashlight's feature options. Logging and geolookup
// refreshing is done consistently as per android-lantern and lantern-desktop. The function does not
// fail, it will always return a reasonable fallback option.
func NewReplicaOptionsGetter(
	f *flashlight.Flashlight,
	getCountry func() (string, error),
) ReplicaOptionsGetter {
	return func() replicaServer.ReplicaOptions {
		var root config.ReplicaOptionsRoot
		if err := f.FeatureOptions(config.FeatureReplica, &root); err != nil {
			log.Errorf(
				"Could not fetch replica feature options: %v",
				err,
			)
			geolookup.Refresh()
			return replicaServer.FallbackReplicaOptions{}
		}
		countryCode, err := getCountry()
		if err != nil {
			log.Errorf("while selecting replica options: error getting country: %v", err)
			log.Debugf("refreshing geolookup")
			geolookup.Refresh()
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
