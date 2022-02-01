See the docs at https://github.com/getlantern/replica-docs.

## On Replica P2P Workflow

Story: https://github.com/getlantern/lantern-internal/issues/5046

Goal of this story is to allow different Replica nodes to act as peer-to-peer nodes that can exchange information amongst each other. One use case we have is to have:
- Replica nodes in free countries (i.e., "Free Peers") to setup a CONNECT proxy and announce a few infohashes to the DHT
- Other Replica nodes in censored countries (i.e., "Censored Peers") to fetch the proxy configurations of the "Free Peers" and proxy traffic through them.

The use cases can extend more than this, but the above is a concrete example of the story.

The breakdown of code between different repos goes like this:
- getlantern/replica's `p2p` package
  - houses the actual DHT communication code (e.g., "announce" and "get-peers" requests)
- getlantern/flashlight's `p2p` package
  - houses a high-level representation of Free and Censored peers (e.g., CensoredP2pCtx and FreeP2pCtx)
    - uses an interface of the necessary functions of getlantern/replica, but doesn't depend on its package
  - The `proxied/p2p*` files contain all the http.RoundTripper logic
- getlantern/lantern-desktop
  - includes both getlantern/replica and getlantern/flashlight packages
  - calls the getlantern/flashlight functions using a concrete implementation of getlantern/replica package, which lives in `p2p/p2pfuncs.go` file

### Tests

A complete test suite that shows this flow is in `getlantern/lantern-desktop/p2p_test.go`.

### Standalone Tooling

There're two standalone tools that can run as standalone "Free" and "Censored" peers, both located in `getlantern/lantern-desktop` repo in `cmd/replica-p2p` directory. Instructions on how to run them is in their respective `main.go` files.
