package replica

import "fmt"

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

func (r *Endpoint) rootUrls() []string {
	return []string{
		// Virtual-hosted-style
		fmt.Sprintf("https://%s.s3.%s.amazonaws.com", r.BucketName, r.AwsRegion),
		// Path-style
		fmt.Sprintf("https://s3.%s.amazonaws.com/%s", r.AwsRegion, r.BucketName),
	}
}
