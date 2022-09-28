See the docs at https://github.com/getlantern/replica-docs

## FAQ

### Search Index Roundtrippers (i.e., how your Replica search query gets fetched)

Replica on the client-side (desktop, Android, etc.) uses two search indices:

- Either a replica-rust instance somewhere remote (e.g., the one at https://replica-search-aws.lantern.io. See [here](https://github.com/getlantern/replica-rust/blob/2cd3443542ed6a8d07774e067eb088f46bac1589/README.md#L53) for a complete breakdown).
  - We'll call this the `primary` search index
- Or, a local "backup" instance living on the user's device, fetched from the DHT
  - See [here](https://github.com/getlantern/replica-docs/blob/c9a8087633de7654d47a0a4d440d6cfcb6cca7b0/README.md#L168) for more info
  - We'll call this the `local` search index

When a search query occurs, `./server/dualsearchroundtripper.go:DualSearchIndexRoundTripper` (which implements `http.RoundTripper`) runs the same request in parallel on both indices (i.e., it multipaths the request) and favours always the `primary` index (for fresher results). See that file for more info.

To be clear, here's the full search code flow:

- `./server/server.go:NewHTTPHandler()` creates a new local HTTP server.
  - Clients like lantern-android and lantern-desktop will create this so that the UI can talk to Go code through HTTP

- A client runs a request like `http://localhost:whateverport/search?s=bunnyfoofoo`
  - Through the `/search` route in [NewHTTPHandler](https://github.com/getlantern/replica/blob/492687d77163b2960755d5f3babbf6e9be4cc20a/server/server.go#L291)
  - which gets handled here in [handleSearch](https://github.com/getlantern/replica/blob/492687d77163b2960755d5f3babbf6e9be4cc20a/server/server.go#L817)

- The request gets routed to [searchProxy.ServeHTTP](https://github.com/getlantern/replica/blob/492687d77163b2960755d5f3babbf6e9be4cc20a/server/server.go#L826)
  - Which strips the prefix route [here](https://github.com/getlantern/replica/blob/492687d77163b2960755d5f3babbf6e9be4cc20a/server/server.go#L258)
  - And continues with the handling [here](https://github.com/getlantern/replica/blob/492687d77163b2960755d5f3babbf6e9be4cc20a/server/proxy.go#L57)
  - And as you see [here](https://github.com/getlantern/replica/blob/492687d77163b2960755d5f3babbf6e9be4cc20a/server/proxy.go#L61), it uses `DualSearchIndexRoundTripper` to run the search query in parallel through both the `primary` and `local` search indices
