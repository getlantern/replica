module github.com/getlantern/replica

go 1.16

require (
	github.com/anacrolix/chansync v0.2.1-0.20210910114620-14955c95ded9 // indirect
	github.com/anacrolix/confluence v1.8.1
	github.com/anacrolix/dht/v2 v2.10.5-0.20210902001729-06cc4fe90e53 // indirect
	github.com/anacrolix/log v0.9.0
	github.com/anacrolix/stm v0.3.0 // indirect
	github.com/anacrolix/torrent v1.30.2
	github.com/aws/aws-sdk-go v1.28.9
	github.com/frankban/quicktest v1.13.1 // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/getlantern/errors v1.0.1
	github.com/getlantern/flashlight v0.0.0-20210922145107-fdcc91512d17
	github.com/getlantern/golog v0.0.0-20210606115803-bce9f9fe5a5f
	github.com/getlantern/meta-scrubber v0.0.1
	github.com/getsentry/sentry-go v0.11.0
	github.com/google/uuid v1.3.0
	github.com/kennygrant/sanitize v1.2.4
	github.com/leanovate/gopter v0.2.9
	github.com/pion/rtp v1.7.2 // indirect
	github.com/rogpeppe/go-internal v1.8.0 // indirect
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20210813211128-0a44fdfbc16e // indirect
)

replace github.com/refraction-networking/utls => github.com/getlantern/utls v0.0.0-20200903013459-0c02248f7ce1

replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go v0.7.1-0.20210422183034-b5805f4c233b

replace github.com/anacrolix/go-libutp => github.com/getlantern/go-libutp v1.0.3-0.20210202003624-785b5fda134e
