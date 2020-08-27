package replica

type Endpoint struct {
	BucketName string
	AwsRegion  string
}

// NewUpload creates a new random uuid or provider+id S3 key prefix to anonymize uploads.
func (r Endpoint) NewUpload(uConfig UploadConfig) Upload {
	uPrefix := uConfig.GetPrefix()

	return Upload{
		UploadPrefix: uPrefix,
		Endpoint:     r,
	}
}
