module github.com/getlantern/replica

go 1.16

require (
	github.com/anacrolix/confluence v1.8.1
	github.com/anacrolix/dht/v2 v2.10.6-0.20211007004332-99263ec9c1c8
	github.com/anacrolix/log v0.10.0
	github.com/anacrolix/torrent v1.35.0
	github.com/aws/aws-sdk-go v1.28.9
	github.com/cloudfoundry/jibber_jabber v0.0.0-20151120183258-bcc4c8345a21 // indirect
	github.com/getlantern/errors v1.0.1
	github.com/getlantern/flashlight v0.0.0-20211213210012-d28066c00d0c
	github.com/getlantern/golog v0.0.0-20210606115803-bce9f9fe5a5f
	github.com/getlantern/meta-scrubber v0.0.1
	github.com/getlantern/proxy v0.0.0-20210806161026-8f52aabf6214 // indirect
	github.com/getsentry/sentry-go v0.11.0
	github.com/google/uuid v1.3.0
	github.com/kennygrant/sanitize v1.2.4
	github.com/leanovate/gopter v0.2.9
	github.com/stretchr/testify v1.7.0
)

replace github.com/refraction-networking/utls => github.com/getlantern/utls v0.0.0-20211116192935-1abdc4b1acab

replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go v0.0.0-20211103152344-c9ce5bfd4854

replace github.com/anacrolix/go-libutp => github.com/getlantern/go-libutp v1.0.3-0.20210202003624-785b5fda134e
