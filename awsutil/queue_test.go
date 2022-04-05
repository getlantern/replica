package awsutil

import (
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sns/snsiface"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
	"github.com/stretchr/testify/assert"
)

func helperLoadBytes(t *testing.T, name string) []byte {
	path := filepath.Join("./", "testdata", name)
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return bytes
}

type mockedSNSClient struct {
	snsiface.SNSAPI
}

type mockedSQSClient struct {
	sqsiface.SQSAPI
}

type mockedSQSClientCreateQueue struct {
	mockedSQSClient
}

func (m mockedSQSClientCreateQueue) CreateQueue(input *sqs.CreateQueueInput) (*sqs.CreateQueueOutput, error) {
	queueUrl := "http://test-queue-url"
	return &sqs.CreateQueueOutput{
		QueueUrl: &queueUrl,
	}, nil
}

type mockedSQSClientReceiveMessage struct {
	mockedSQSClient
	messages []string
}

func (m mockedSQSClientReceiveMessage) ReceiveMessage(input *sqs.ReceiveMessageInput) (*sqs.ReceiveMessageOutput, error) {
	var messages []*sqs.Message
	for _, message := range m.messages {
		messageString := message
		messages = append(messages, &sqs.Message{
			Body: &messageString,
		})
	}
	return &sqs.ReceiveMessageOutput{
		Messages: messages,
	}, nil
}

func (m mockedSQSClientReceiveMessage) DeleteMessage(input *sqs.DeleteMessageInput) (*sqs.DeleteMessageOutput, error) {
	return &sqs.DeleteMessageOutput{}, nil
}

type mockedSNSClientSubscribe struct {
	snsiface.SNSAPI
}

func (m mockedSNSClientSubscribe) Subscribe(input *sns.SubscribeInput) (*sns.SubscribeOutput, error) {
	subscriptionArn := "test-sns-arn"
	return &sns.SubscribeOutput{
		SubscriptionArn: &subscriptionArn,
	}, nil
}

func (m mockedSQSClientCreateQueue) GetQueueAttributes(input *sqs.GetQueueAttributesInput) (*sqs.GetQueueAttributesOutput, error) {
	attributeArn := "test-queue-arn"
	attributes := map[string]*string{
		"QueueArn": &attributeArn,
	}

	return &sqs.GetQueueAttributesOutput{Attributes: attributes}, nil
}

func TestQueueCreateQueue(t *testing.T) {
	queue := Queue{
		SQSClient: mockedSQSClientCreateQueue{},
		SNSClient: mockedSNSClient{},
	}

	if _, err := queue.createQueue(); err != nil {
		t.Errorf("failed to create queue: %v", err)
	}
}

func TestQueueSubscribe(t *testing.T) {
	queue := Queue{
		SQSClient: mockedSQSClient{},
		SNSClient: mockedSNSClientSubscribe{},
	}

	if err := queue.subscribeQueue("test-arn"); err != nil {
		t.Errorf("failed to subscribe to queue: %v", err)
	}
}

func TestQueueInit(t *testing.T) {
	queue := Queue{
		SQSClient: mockedSQSClientCreateQueue{},
		SNSClient: mockedSNSClientSubscribe{},
	}

	if err := queue.Init(); err != nil {
		t.Errorf("failed to initialize to queue: %v", err)
	}
}

func TestQueueGetMessage(t *testing.T) {
	s3Events := []S3Event{}
	handler := func(s3Event S3Event) error {
		t.Logf("calling handler with %+v", s3Event)
		s3Events = append(s3Events, s3Event)
		return nil
	}

	putEventBody := string(helperLoadBytes(t, "put-object-event.json"))
	deleteEventBody := string(helperLoadBytes(t, "delete-object-event.json"))
	client := mockedSQSClientReceiveMessage{
		messages: []string{putEventBody, deleteEventBody},
	}

	queue := Queue{
		SQSClient:   client,
		SNSClient:   mockedSNSClientSubscribe{},
		sqsQueue:    &SQSQueue{},
		initialized: true,
	}

	if err := queue.GetMessage(handler); err != nil {
		t.Errorf("failed to get message: %v", err)
	}
	t.Logf("s3Events: %+v", s3Events)
	actualPut := s3Events[0]

	assert.Equal(t, actualPut.EventName, "ObjectCreated:Put")
	assert.Equal(t, *actualPut.S3.Object.Key, "74435c9f-8ed9-4d25-84e8-517e94ae62be/test-image.jpeg")

	if err := queue.GetMessage(handler); err != nil {
		t.Errorf("failed to get message: %v", err)
	}
	actualDelete := s3Events[1]
	assert.Equal(t, actualDelete.EventName, "ObjectRemoved:Delete")
	assert.Equal(t, *actualDelete.S3.Object.Key, "74435c9f-8ed9-4d25-84e8-517e94ae62be/image-test.jpeg")
}
