module github.com/getlantern/replica

go 1.16

require (
	github.com/anacrolix/confluence v1.9.0
	github.com/anacrolix/dht/v2 v2.15.1
	github.com/anacrolix/go-libutp v1.0.5 // indirect
	github.com/anacrolix/log v0.10.0
	github.com/anacrolix/torrent v1.35.1-0.20211104090255-eaeb38b18c6a
	github.com/aws/aws-sdk-go v1.28.9
	github.com/getlantern/borda v0.0.0-20211118145443-aeeab8933313
	github.com/getlantern/errors v1.0.1
	github.com/getlantern/eventual v1.0.0 // indirect
	github.com/getlantern/golog v0.0.0-20210606115803-bce9f9fe5a5f
	github.com/getlantern/meta-scrubber v0.0.1
	github.com/getlantern/ops v0.0.0-20200403153110-8476b16edcd6
	github.com/getsentry/sentry-go v0.11.0
	github.com/google/uuid v1.3.0
	github.com/kennygrant/sanitize v1.2.4
	github.com/leanovate/gopter v0.2.9
	github.com/stretchr/testify v1.7.0
	golang.org/x/crypto v0.0.0-20211108221036-ceb1ce70b4fa // indirect
	golang.org/x/net v0.0.0-20211111160137-58aab5ef257a // indirect
)

replace github.com/lucas-clemente/quic-go => github.com/getlantern/quic-go v0.0.0-20211103152344-c9ce5bfd4854

replace github.com/refraction-networking/utls => github.com/getlantern/utls v0.0.0-20211116192935-1abdc4b1acab
