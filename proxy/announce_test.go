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
	var ih [20]byte
	_, err = cryptoRand.Read(ih[:])
	require.NoError(t, err)
	port := mathRand.Intn(49151 - 1024)
	pubIpCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	myPublicIp, err := publicip.Get4(pubIpCtx)
	require.NoError(t, err)

	// Announce infohash
	err = Announce([]*dht.Server{s}, [][20]byte{ih}, port)
	require.NoError(t, err)

	// Get Peers for the same infohash
	getPeersCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	peersChan := make(chan dht.Peer, 128)
	err = GetPeers(getPeersCtx, []*dht.Server{s}, [][20]byte{ih}, peersChan)
	// XXX <08-01-22, soltzen> DeadlineExceeded is fine here: this just means
	// we've collected as much as we can with the time we have
	if err != nil && err != context.DeadlineExceeded {
		require.Fail(t, "getPeers failed because: %v", err)
	}

	// Drain channel into a map to make sure we have unique peers
	peers := map[string]dht.Peer{}
	for p := range peersChan {
		peers[p.String()] = p
	}

	// Assert we got a peer with the same public IP as us and the same port we
	// specified.
	// XXX <13-01-22, soltzen> Don't check if len(peers) == 1; you'll always
	// get jokers replying to all your DHT requests
	require.NotEmpty(t, peers)
	atLeastOnePeerMatches := false
	for _, peer := range peers {
		if myPublicIp.String() == peer.IP.String() {
			atLeastOnePeerMatches = true
			require.Equal(t, port, peer.Port)
		}
	}
	require.True(t, atLeastOnePeerMatches)
}
