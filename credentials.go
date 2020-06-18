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
	SessionRegion  string
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

func (cp *cognitoProvider) sessionRegion() string {
	if cp.SessionRegion == "" {
		return DefaultEndpoint.AwsRegion
	}
	return cp.SessionRegion
}

func (cp *cognitoProvider) newCredentials() (*cognitoidentity.Credentials, error) {
	svc := cognitoidentity.New(session.New(), aws.NewConfig().WithRegion(cp.sessionRegion()))
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

func (creds *cognitoProvider) getCredentials() (*credentials.Credentials, error) {
	creds.Lock()
	defer creds.Unlock()
	if creds.IsExpired() {
		if cr, err := creds.newCredentials(); err != nil {
			return nil, err
		} else {
			creds.value = credentials.Value{
				AccessKeyID:     *cr.AccessKeyId,
				SecretAccessKey: *cr.SecretKey,
				SessionToken:    *cr.SessionToken,
			}
			creds.SetExpiration(*cr.Expiration, 20*time.Second)
		}
	}
	return credentials.NewCredentials(creds), nil
}
