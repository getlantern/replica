package replica

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"path/filepath"
	"sync/atomic"
	"time"

	_ "github.com/anacrolix/envpprof"
	"github.com/anacrolix/torrent/metainfo"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cognitoidentity"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"golang.org/x/xerrors"

	"github.com/google/uuid"
)

var creds atomic.Value

const (
	bucket = "getlantern-replica"
	region = "ap-southeast-1"
)

type cognitoprovider struct {
	credentials.Expiry
	value credentials.Value
	//         ...
}

func (cp *cognitoprovider) Retrieve() (credentials.Value, error) {
	return cp.value, nil
}

func newCredentials() (*cognitoprovider, error) {
	svc := cognitoidentity.New(session.New(), aws.NewConfig().WithRegion(region))
	idRes, err := svc.GetId(&cognitoidentity.GetIdInput{
		IdentityPoolId: aws.String("ap-northeast-1:d13f20ba-1358-42ba-898d-6f26847f07a9"),
	})

	if err != nil {
		return nil, err
	}

	credRes, err := svc.GetCredentialsForIdentity(&cognitoidentity.GetCredentialsForIdentityInput{
		IdentityId: idRes.IdentityId,
		//IdentityId: aws.String("ap-northeast-1:d13f20ba-1358-42ba-898d-6f26847f07a9"),
	})
	expiry := &cognitoprovider{
		value: credentials.Value{
			AccessKeyID:     *credRes.Credentials.AccessKeyId,
			SecretAccessKey: *credRes.Credentials.SecretKey,
			SessionToken:    *credRes.Credentials.SessionToken,
		},
	}
	expiry.SetExpiration(*credRes.Credentials.Expiration, 20*time.Second)
	return expiry, nil
}

func getCredentials() (*credentials.Credentials, error) {
	if creds.Load() != nil {
		if creds.Load().(*cognitoprovider).Expiry.IsExpired() {
			if cr, err := newCredentials(); err != nil {
				return nil, err
			} else {
				creds.Store(cr)
			}
		}
	}
	return credentials.NewCredentials(creds.Load().(*cognitoprovider)), nil
}

func newSession() *session.Session {
	creds, err := getCredentials()
	if err != nil {
		return session.New()
	}

	return session.Must(session.NewSession(&aws.Config{
		Credentials: creds,
		Region:      aws.String(region),
		//CredentialsChainVerboseErrors: aws.Bool(true),
	}))
}

func NewPrefix() string {
	u, err := uuid.NewRandom()
	if err != nil {
		panic(err)
	}
	return u.String()
}

func Upload(f io.Reader, s3Key string) error {
	sess := newSession()
	uploader := s3manager.NewUploader(sess)
	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(s3Key),
		Body:   f,
	})
	if err != nil {
		return xerrors.Errorf("uploading to s3: %w", err)
	}
	return nil
}

func UploadFile(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", xerrors.Errorf("opening file %q: %w", filename, err)
	}
	defer f.Close()
	s3Key := path.Join(NewPrefix(), filepath.Base(filename))
	return s3Key, Upload(f, s3Key)
}

func DeleteFile(s3key string) error {
	svc := s3.New(newSession())
	input := &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(s3key),
	}
	_, err := svc.DeleteObject(input)
	return err
}

// Returns the object metainfo for the given key.
func GetObjectTorrent(key string) (io.ReadCloser, error) {
	sess := newSession()
	svc := s3.New(sess)
	out, err := svc.GetObjectTorrent(&s3.GetObjectTorrentInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

// Downloads the metainfo for the Replica object to a .torrent file in the current working directory.
func GetTorrent(key string) error {
	t, err := GetObjectTorrent(key)
	if err != nil {
		return xerrors.Errorf("getting object torrent: %w", err)
	}
	defer t.Close()
	f, err := os.OpenFile(path.Base(key)+".torrent", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0640)
	if err != nil {
		return xerrors.Errorf("opening output file: %w", err)
	}
	log.Printf("created %q", f.Name())
	defer f.Close()
	if _, err := io.Copy(f, t); err != nil {
		return xerrors.Errorf("copying torrent: %w", err)
	}
	if err := f.Close(); err != nil {
		return xerrors.Errorf("closing torrent file: %w", f.Close())
	}
	return nil
}

// Walks the torrent files stored in the directory.
func IterUploads(dir string, f func(mi *metainfo.MetaInfo, err error)) error {
	entries, err := ioutil.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		mi, err := metainfo.LoadFromFile(p)
		if err != nil {
			f(nil, fmt.Errorf("loading metainfo from file %q: %w", p, err))
			continue
		}
		f(mi, nil)
	}
	return nil
}
