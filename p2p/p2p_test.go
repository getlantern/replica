package proxy

import (
	"context"
	cryptoRand "crypto/rand"
	mathRand "math/rand"
	"testing"
	"time"

	"github.com/anacrolix/dht/v2"
	"github.com/anacrolix/publicip"
	"github.com/stretchr/testify/require"
)

// TestP2pNetworking runs two DHT operations:
// - one to announce a random infohash to the network
// - and, one to get all the announcing peers for that network.
// The test asserts that the address we've announced is the one we fetch.
func TestP2pNetworking(t *testing.T) {
	// Init Dht server
	cfg := dht.NewDefaultServerConfig()
	cfg.NoSecurity = false
	s, err := dht.NewServer(cfg)
	require.NoError(t, err)

	// Get a random infohash, a random port and our public ip
	ihs := make([][20]byte, 2)
	for i := range ihs {
		_, err = cryptoRand.Read(ihs[i][:])
		require.NoError(t, err)
	}
	port := mathRand.Intn(49152-1024) + 1024
	pubIpCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	myPublicIp, err := publicip.Get4(pubIpCtx)
	require.NoError(t, err)

	// Announce infohashes
	t.Logf("announcing %x", ihs)
	err = Announce([]*dht.Server{s}, ihs, port)
	require.NoError(t, err)

	// Get Peers for the same infohash
	getPeersCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	peersChan := make(chan Peer)
	matchedInfohashes := 0
	go func() {
		doneInfohashes := make(map[[20]byte]struct{})
		for peer := range peersChan {
			t.Logf("got peer %v", peer)
			if _, ok := doneInfohashes[peer.Infohash]; ok {
				continue
			}
			// Hurry up net/netip! Assert we got a peer with the same public IP as us and the same
			// port we specified. XXX <13-01-22, soltzen> Don't assume we're the only peer for a
			// random infohash; you'll always get jokers replying to all your DHT requests.
			if myPublicIp.String() != peer.IP.String() {
				continue
			}
			// Rather than fail on a secondary test goroutine, just filter out this invalid item.
			if peer.Port != port {
				continue
			}
			matchedInfohashes++
			doneInfohashes[peer.Infohash] = struct{}{}
		}
	}()
	t.Logf("getting peers")
	err = GetPeers(getPeersCtx, []*dht.Server{s}, ihs, peersChan)
	// XXX <08-01-22, soltzen> DeadlineExceeded is fine here: this just means
	// we've collected as much as we can with the time we have
	if err != nil && err != context.DeadlineExceeded {
		require.Fail(t, "getPeers failed because: %v", err)
	}

	// We should see ourselves at least once for each infohash.
	require.Equal(t, len(ihs), matchedInfohashes)
}
