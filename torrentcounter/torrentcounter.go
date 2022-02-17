// package torrentcounter provides a "Reference Counting" strategy to associate
// a torrent.PeerConn (i.e., a torrent peer connection) with a torrent.InfoHash
// and only drop it when there are no more peer connections for that torrent.
//
// So, assume there are 3 peer connections (pc1, pc2 and pc3) for torrent with infohash AAAAAA.
// You'll need to run the following when you get a new peer connection (See example here: https://github.com/getlantern/replica-peer/blob/3d622e3fe0c6432f98eab9332b8c543181960956/main.go#L333):
//
// - AddPeerConn(pc1, AAAAAA)
// - AddPeerConn(pc2, AAAAAA)
// - AddPeerConn(pc3, AAAAAA)
//
// This would add 3 peer connections to AAAAAA.
//
// Later, call DropPeerConn() when the PeerConn is closed.
//
// When you call DropPeerConn() on **all** peer connections for AAAAAA,
// TorrentCounter.onTorrentDropFunc() callback will trigger, giving the caller
// a chance to remove the torrent from the torrent client, for example.
//
// This structure is thread-safe.
package torrentcounter

import (
	"fmt"
	"sync"

	"github.com/anacrolix/torrent"
	"github.com/getlantern/golog"
)

var log = golog.LoggerFor("replica-torrentcounter")

type TorrentCounter struct {
	mu                    sync.Mutex
	infohashToCounterMap  map[torrent.InfoHash]int
	peerConnToInfohashMap map[*torrent.PeerConn]torrent.InfoHash
	onTorrentDropFunc     func(torrent.InfoHash)
}

// New creates a new TorrentCounter.
//
// onTorrentDropFunc is called when all torrent.PeerConn for a specific
// infohash are dropped (i.e., DropPeerConn is called for all PeerConns
// associated with an infohash)
func New(onTorrentDropFunc func(torrent.InfoHash)) *TorrentCounter {
	return &TorrentCounter{
		infohashToCounterMap:  map[torrent.InfoHash]int{},
		peerConnToInfohashMap: map[*torrent.PeerConn]torrent.InfoHash{},
		onTorrentDropFunc:     onTorrentDropFunc,
	}
}

func (s *TorrentCounter) AddPeerConn(pc *torrent.PeerConn, ih torrent.InfoHash) {
	s.mu.Lock()
	defer s.mu.Unlock()
	log.Debugf("AddPeerConn to infohash %v", ih.HexString())
	s.infohashToCounterMap[ih]++
	s.peerConnToInfohashMap[pc] = ih
}

func (s *TorrentCounter) DropPeerConn(pc *torrent.PeerConn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Fetch infohash
	ih, ok := s.peerConnToInfohashMap[pc]
	if !ok {
		// We've already forgotten about the PeerConn, such as when we fail to
		// add the torrent in the first place, and have already unregistered
		// it. FWIW the PeerConn close hook in anacrolix/torrent probably
		// doesn't get called multiple times.
		return
	}
	delete(s.peerConnToInfohashMap, pc)
	log.Debugf("DropPeerConn to infohash %v", ih.HexString())
	// Decrement the infohash reference
	s.infohashToCounterMap[ih]--
	newCounter := s.infohashToCounterMap[ih]
	switch {
	case newCounter < 0:
		panic(
			fmt.Sprintf(
				"Logic error: found a infohash [%v] in s.infohashToCounterMap [%+v] that has negative number of PeerConn: [%v]",
				ih.HexString(), s.infohashToCounterMap, newCounter))
	case newCounter == 0:
		if s.onTorrentDropFunc != nil {
			s.onTorrentDropFunc(ih)
		}
		delete(s.infohashToCounterMap, ih)
	default:
		log.Debugf("Dropping PeerConn to torrent (ih: %v). %v PeerConn left", ih, newCounter)
	}
}

func (s *TorrentCounter) TorrentLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.infohashToCounterMap)
}

func (s *TorrentCounter) PeerConnLen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.peerConnToInfohashMap)
}
