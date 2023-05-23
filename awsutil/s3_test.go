package awsutil

import (
	"context"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/leanovate/gopter"
	"github.com/leanovate/gopter/gen"
	"github.com/leanovate/gopter/prop"
	"github.com/stretchr/testify/assert"
)

type mockedS3Client struct {
	s3iface.S3API
	objects        []*s3.Object
	objectsPerPage int
	t              *testing.T
}

func (m mockedS3Client) ListObjects(input *s3.ListObjectsInput) (*s3.ListObjectsOutput, error) {
	return &s3.ListObjectsOutput{
		Contents: m.objects,
	}, nil
}

func (m mockedS3Client) ListObjectsPagesWithContext(ctx aws.Context, input *s3.ListObjectsInput, fn func(*s3.ListObjectsOutput, bool) bool, opts ...request.Option) error {
	aggregated := []*s3.Object{}
	for i, obj := range m.objects {
		aggregated = append(aggregated, obj)
		if len(aggregated) == m.objectsPerPage {
			output := &s3.ListObjectsOutput{
				Contents: aggregated,
			}
			cont := fn(output, i == len(m.objects)-1)
			aggregated = []*s3.Object{}
			if !cont {
				break
			}
		}
	}

	if len(aggregated) > 0 {
		output := &s3.ListObjectsOutput{
			Contents: aggregated,
		}
		fn(output, true)
	}

	return nil
}

func TestS3GetAllObjectsManually(t *testing.T) {
	m := mockedS3Client{
		objectsPerPage: 1,
		objects: []*s3.Object{
			{
				Key: aws.String("key1"),
			},
			{
				Key: aws.String("key2"),
			},
		},
		t: t,
	}

	var actual []*s3.Object
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := ForAllObjects(ctx, m, "test-bucket", func(object *s3.Object) bool {
		actual = append(actual, object)
		return true
	})
	assert.NoErrorf(t, err, "failed to list objects: %v", err)
	assert.Equal(t, aws.StringValue(actual[0].Key), "key1")
	assert.Equal(t, aws.StringValue(actual[1].Key), "key2")
}

func genS3GetAllObjectsCheck(t *testing.T) func([]*s3.Object, int) bool {
	return func(objects []*s3.Object, perPage int) bool {
		m := mockedS3Client{
			objectsPerPage: perPage,
			objects:        objects,
			t:              t,
		}

		var actual []*s3.Object
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		err := ForAllObjects(ctx, m, "test-bucket", func(object *s3.Object) bool {
			actual = append(actual, object)
			return true
		})
		assert.NoErrorf(t, err, "failed to list objects: %v", err)

		return assert.ElementsMatchf(t, m.objects, actual, "listed objects match original")
	}
}

func TestS3GetAllObjectsAutomatically(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode.")
	}

	properties := gopter.NewProperties(nil)

	objectGen := gen.StructPtr(reflect.TypeOf(s3.Object{}), map[string]gopter.Gen{
		"Key": gen.AnyString().Map(func(k string) *string {
			return &k
		}),
	})

	properties.Property("works for numObjects <= 10000, pageSize <= 1000", prop.ForAll(
		genS3GetAllObjectsCheck(t),
		gen.IntRange(1, 10000).FlatMap(func(n interface{}) gopter.Gen {
			size := n.(int)
			return gen.SliceOfN(size, objectGen)
		}, reflect.SliceOf(reflect.TypeOf([]*s3.Object{}))),
		gen.IntRange(1, 1000),
	))

	properties.TestingRun(t)
}

func TestGetPrefixFromTorrentKey(t *testing.T) {
	var (
		prefix string
		err    error
	)

	prefix, err = GetPrefixFromTorrentKey("9a49efd4-4a1f-431d-9619-1ac2aaefff63/torrent")
	assert.Nil(t, err)
	assert.Equal(t, "9a49efd4-4a1f-431d-9619-1ac2aaefff63", prefix)

	prefix, err = GetPrefixFromTorrentKey("youtube_v2-RWk3SmRgE1E/torrent")
	assert.Nil(t, err)
	assert.Equal(t, "youtube_v2-RWk3SmRgE1E", prefix)

	prefix, err = GetPrefixFromTorrentKey("9a49efd4-4a1f-431d-9619-1ac2aaefff63/data/afile.mp4")
	assert.Error(t, err)
	prefix, err = GetPrefixFromTorrentKey("youtube_v2-RWk3SmRgE1E/data/afile.mp4")
	assert.Error(t, err)
}
