package proxy

import (
	"context"
	"fmt"
	"sync"

	"github.com/anacrolix/dht/v2"
)

// Announces a proxy at the local port on all the servers to all the infohashes. Returns when done.
func Announce(ss []*dht.Server, ihs [][20]byte, port int) error {
	for _, s := range ss {
		for _, ih := range ihs {
			a, err := s.AnnounceTraversal(ih, dht.AnnouncePeer(dht.AnnouncePeerOpts{
				Port:        port,
				ImpliedPort: false,
			}))
			if err != nil {
				return fmt.Errorf("announcing to %x on %v", ih, s)
			}
			// Drain the Peers channel, we don't use it.
			for range a.Peers {
			}
			a.Close()
		}
	}
	return nil
}

// Sends peers found by any of the servers for any of the info-hashes to the channel. A single
// traversal is initiated for each combination of server and info-hash. There can be duplicate
// peers. Returns when all traversals are exhausted or there's an error initiating a traversal.
func GetPeers(ctx context.Context, ss []*dht.Server, ihs [][20]byte, peers chan<- dht.Peer) error {
	var wg sync.WaitGroup
	for _, s := range ss {
		for _, ih := range ihs {
			a, err := s.AnnounceTraversal(ih)
			if err != nil {
				return fmt.Errorf("announcing to %x on %v", ih, s)
			}
			defer a.Close()
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer a.Close()
				for pv := range a.Peers {
					for _, p := range pv.Peers {
						select {
						case peers <- p:
						case <-ctx.Done():
							return
						}
					}
				}
			}()
		}
	}
	wg.Wait()
	return ctx.Err()
}
