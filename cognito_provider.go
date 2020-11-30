package replica

import (
	"sync"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cognitoidentity"
)

type cognitoProvider struct {
	IdentityPoolId *string
	credentials.Expiry
	value credentials.Value
	sync.Mutex
}

func (cp *cognitoProvider) Retrieve() (credentials.Value, error) {
	return cp.value, nil
}

func (cp *cognitoProvider) identityPoolId() *string {
	if cp.IdentityPoolId != nil {
		return cp.IdentityPoolId
	}
	return aws.String("ap-southeast-1:0b509375-33f5-43f8-97c3-8ee7db4c5c14")
}

func (cp *cognitoProvider) newCredentials(region string) (*cognitoidentity.Credentials, error) {
	svc := cognitoidentity.New(session.New(), aws.NewConfig().WithRegion(region))
	idRes, err := svc.GetId(&cognitoidentity.GetIdInput{
		IdentityPoolId: cp.identityPoolId(),
	})

	if err != nil {
		return nil, err
	}

	credRes, err := svc.GetCredentialsForIdentity(&cognitoidentity.GetCredentialsForIdentityInput{
		IdentityId: idRes.IdentityId,
	})
	return credRes.Credentials, nil
}

func (cp *cognitoProvider) getCredentials(region string) (*credentials.Credentials, error) {
	cp.Lock()
	defer cp.Unlock()
	if cp.IsExpired() {
		if cr, err := cp.newCredentials(region); err != nil {
			return nil, err
		} else {
			cp.value = credentials.Value{
				AccessKeyID:     *cr.AccessKeyId,
				SecretAccessKey: *cr.SecretKey,
				SessionToken:    *cr.SessionToken,
			}
			cp.SetExpiration(*cr.Expiration, 20*time.Second)
		}
	}
	return credentials.NewCredentials(cp), nil
}
