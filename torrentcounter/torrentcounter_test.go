package torrentcounter

import (
	cryptoRand "crypto/rand"
	"crypto/sha1"
	mathRand "math/rand"
	"sync"
	"testing"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/stretchr/testify/require"
)

func TestTorrentCounter(t *testing.T) {
	mathRand.Seed(time.Now().Unix())

	// Get a bunch of random infohashes
	randomInfohashes := func(limit int) []torrent.InfoHash {
		ihs := []torrent.InfoHash{}
		for i := 0; i < limit; i++ {
			b := make([]byte, 16)
			_, err := cryptoRand.Read(b)
			require.NoError(t, err)
			ihs = append(ihs, torrent.InfoHash(sha1.Sum(b)))
		}
		return ihs
	}(5)

	t.Run("Test flow non-deterministically", func(t *testing.T) {
		tc := New(
			func(torrent.InfoHash) {
				// Assume this operation takes a bit of time
				time.Sleep(time.Duration(mathRand.Intn(2)) * time.Second)
			})

		// - Add a high number of concurrent PeerConns to one of our infohashes
		// - Wait a bit
		// - Then drop each one
		var wg sync.WaitGroup
		for i := 0; i < 1000; i++ {
			wg.Add(1)
			rih := randomInfohashes[mathRand.Intn(len(randomInfohashes))]
			go func() {
				pc := &torrent.PeerConn{}
				tc.AddPeerConn(pc, rih)
				time.Sleep(time.Duration(mathRand.Intn(2)) * time.Second)
				tc.DropPeerConn(pc)
				wg.Done()
			}()
		}
		wg.Wait()

		// Assert that there are no more PeerConns and for all dropped PeerConns,
		// their corresponding torrents dropped as well
		require.Equal(t, 0, tc.TorrentLen())
		require.Equal(t, 0, tc.PeerConnLen())
	})

	t.Run("Assert that an infohash would drop only when all PeerConns are dropped",
		func(t *testing.T) {
			tc := New(
				func(torrent.InfoHash) {
					// Assume this operation takes a bit of time
					time.Sleep(time.Duration(mathRand.Intn(2)) * time.Second)
				})

			// Declare a few PeerConns
			pcs := []*torrent.PeerConn{
				&torrent.PeerConn{},
				&torrent.PeerConn{},
				&torrent.PeerConn{},
				&torrent.PeerConn{},
			}
			// Add them to a random infohash (doesn't matter which one)
			for _, pc := range pcs {
				tc.AddPeerConn(pc, randomInfohashes[0])
			}

			// Start dropping them
			for i, pc := range pcs {
				tc.DropPeerConn(pc)

				if i == len(pcs)-1 {
					// Assert that the torrent drops if this is the last element in the list
					require.Equal(t, 0, tc.TorrentLen())
				} else {
					// Else, assert we still have our infohash if not all PeerConns dropped
					require.Equal(t, 1, tc.TorrentLen())
				}
			}
		})
}
