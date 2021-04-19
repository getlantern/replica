package replica

import "fmt"

const (
	StorageProviderS3      = "s3"
	StorageProviderTencent = "tencent"
)

var (
	DefaultEndpoint = Endpoint{
		StorageProvider: StorageProviderS3,
		BucketName:      "getlantern-replica",
		Region:          "ap-southeast-1",
	}
	DefaultMetadataEndpoint = Endpoint{
		StorageProvider: StorageProviderS3,
		BucketName:      "replica-metadata",
		Region:          "ap-southeast-1",
	}
)

type Endpoint struct {
	StorageProvider string
	Region          string
	BucketName      string
}

// NewUpload creates a new random uuid or provider+id S3 key prefix to anonymize uploads.
func (r Endpoint) NewUpload(uConfig UploadConfig) Upload {
	return Upload{
		UploadPrefix: uConfig.GetPrefix(),
		Endpoint:     r,
	}
}

func (r *Endpoint) rootUrls() []string {
	// TODO: Refactor this to use an interface or template and not a switch.
	switch r.StorageProvider {
	case StorageProviderS3:
		return []string{
			// Virtual-hosted-style
			fmt.Sprintf("https://%s.s3.%s.amazonaws.com", r.BucketName, r.Region),
			// Path-style
			fmt.Sprintf("https://s3.%s.amazonaws.com/%s", r.Region, r.BucketName),
		}
	case StorageProviderTencent:
		return []string{
			// Virtual-hosted-style
			fmt.Sprintf("https://%s.cos.%s.myqcloud.com", r.BucketName, r.Region),
			// Path-style
			fmt.Sprintf("https://cos.%s.myqcloud.com/%s", r.Region, r.BucketName),
		}
	default:
		return []string{}
	}
}
