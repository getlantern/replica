module github.com/getlantern/replica

go 1.16

require (
	github.com/anacrolix/confluence v1.8.1
	github.com/anacrolix/dht/v2 v2.10.6-0.20211007004332-99263ec9c1c8
	github.com/anacrolix/log v0.10.0
	github.com/anacrolix/torrent v1.35.0
	github.com/aws/aws-sdk-go v1.28.9
	github.com/getlantern/errors v1.0.1
	github.com/getlantern/flashlight v0.0.0-20210922145107-fdcc91512d17
	github.com/getlantern/golog v0.0.0-20210606115803-bce9f9fe5a5f
	github.com/getlantern/meta-scrubber v0.0.1
	github.com/getsentry/sentry-go v0.11.0
	github.com/google/uuid v1.3.0
	github.com/kennygrant/sanitize v1.2.4
	github.com/leanovate/gopter v0.2.9
	github.com/stretchr/testify v1.7.0
	golang.org/x/net v0.0.0-20211020060615-d418f374d309 // indirect
)

replace github.com/refraction-networking/utls => github.com/getlantern/utls v0.0.0-20200903013459-0c02248f7ce1

replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go v0.7.1-0.20210422183034-b5805f4c233b

replace github.com/anacrolix/go-libutp => github.com/getlantern/go-libutp v1.0.3-0.20210202003624-785b5fda134e

// replace github.com/anacrolix/torrent => /home/soltzen/dev/own/anacrolix_torrent
