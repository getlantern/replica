Replica uses [magnet links](https://en.wikipedia.org/wiki/Magnet_URI_scheme) to describe content. There are two categories of content typically referenced: Replica uploads onto S3, and BitTorrent files in the public BitTorrent network.

Parameter|Example|Notes
-|-|-
`xt`|`urn:btih:3e36296a9a6e8c55c299d08a3a176fb9d0b3f3c8`|We use the BitTorrent infohash encoded in the standard form, `urn:btih:[BitTorrent info hash encoded in hex]`.
`tr`|http://s3-tracker.ap-southeast-1.amazonaws.com:6969/announce|We currently include the S3 tracker for the Replica bucket, and no trackers otherwise. Down the track we may want to include some global trackers, or add them implicitly to the Replica users client where appropriate (and maybe "export" links with a few global trackers for non-Replica usability).
`so`|0|Replica deals with individual files, and torrents can contain more than one. The `select only` parameter is used with traditional clients to specify which files to download. We use it to reference which file inside a torrent operations should be performed on. This gives best-case compatibility with regular BitTorrent clients.
`dn`|Lawandkermit.mp4|This field normally provides the name field in a torrent info before the info has been made available, or just a prettier name to use when referring to a torrent. We use it with the name of the specific file referenced in the torrent, and without the UUID prefix on torrent names that are hosted on S3.
`as`|https://getlantern-replica.s3-ap-southeast-1.amazonaws.com/35ca7a3f-a5f6-4dad-9881-57131fe007b6/Lawandkermit.mp4|This is a valid source to retrieve the torrent content directly. For S3 uploads, we provide the full HTTP URI, for non-BitTorrent and HTTP-enabled BitTorrent clients.
`xs`|replica:35ca7a3f-a5f6-4dad-9881-57131fe007b6/Lawandkermit.mp4|Here we use a custom, extensible URI for S3 uploads of the form `replica:[s3 key]`.
