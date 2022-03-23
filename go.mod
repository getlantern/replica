module github.com/getlantern/replica

go 1.16

require (
	crawshaw.io/sqlite v0.3.3-0.20210127221821-98b1f83c5508
	github.com/anacrolix/confluence v1.9.0
	github.com/anacrolix/dht/v2 v2.16.2-0.20220311024416-dd658f18fd51
	github.com/anacrolix/log v0.13.1
	github.com/anacrolix/publicip v0.2.0
	github.com/anacrolix/torrent v1.41.1-0.20220315024234-5a61d8f6ac93
	github.com/aws/aws-sdk-go v1.28.9
	github.com/getlantern/borda v0.0.0-20211118145443-aeeab8933313
	github.com/getlantern/dhtup v0.0.0-20220328110708-54c1a983abef
	github.com/getlantern/errors v1.0.1
	github.com/getlantern/eventual/v2 v2.0.2
	github.com/getlantern/golog v0.0.0-20211223150227-d4d95a44d873
	github.com/getlantern/meta-scrubber v0.0.1
	github.com/getlantern/ops v0.0.0-20200403153110-8476b16edcd6
	github.com/getsentry/sentry-go v0.11.0
	github.com/google/uuid v1.3.0
	github.com/gorilla/mux v1.8.0
	github.com/kennygrant/sanitize v1.2.4
	github.com/leanovate/gopter v0.2.9
	github.com/stretchr/testify v1.7.0
)

replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go v0.0.0-20211103152344-c9ce5bfd4854

replace github.com/refraction-networking/utls => github.com/getlantern/utls v0.0.0-20211116192935-1abdc4b1acab

// replace github.com/getlantern/dhtup => ../dhtup
