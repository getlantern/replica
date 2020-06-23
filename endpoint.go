package replica

import "github.com/google/uuid"

type Endpoint struct {
	BucketName string
	AwsRegion  string
}

// NewUpload creates a new random S3 key prefix to anonymize uploads.
func (r Endpoint) NewUpload() Upload {
	u, err := uuid.NewRandom()
	if err != nil {
		panic(err)
	}
	return Upload{
		UploadPrefix: UploadPrefix{u},
		Endpoint:     r,
	}
}
