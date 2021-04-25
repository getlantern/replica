package awsutil

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/sns"
	"github.com/aws/aws-sdk-go/service/sns/snsiface"
	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/aws/aws-sdk-go/service/sqs/sqsiface"
	"github.com/google/uuid"
)

var s3PrefixRegex = regexp.MustCompile("^(.*)/torrent$")

// ForAllObjects will iterate through all of the objects in a specified bucket and call the perObj function with their results
// If the perObj function returns false, it will stop iterating
func ForAllObjects(ctx context.Context, s3Client s3iface.S3API, bucket string, perObj func(object *s3.Object) (moar bool)) error {
	return s3Client.ListObjectsPagesWithContext(ctx, &s3.ListObjectsInput{
		Bucket: &bucket,
	}, func(output *s3.ListObjectsOutput, lastPage bool) bool {
		for _, object := range output.Contents {
			if !perObj(object) {
				return false
			}
		}
		return true
	})
}

// GetPrefixFromTorrentKey gets the S3 path prefix from a torrent object in the replica bucket
func GetPrefixFromTorrentKey(key string) (string, error) {
	matches := s3PrefixRegex.FindStringSubmatch(key)
	if len(matches) == 2 {
		return matches[1], nil
	} else {
		return "", fmt.Errorf("non-torrent s3 key: %v", key)
	}
}

// S3Event represents a newly added or deleted S3 object
type S3Event struct {
	EventName string `json:"eventName"`
	S3        *struct {
		Object *struct {
			Key *string `json:"key"`
		} `json:"object"`
	} `json:"s3"`
}

type SQSMessageBody struct {
	Message string
}

type SQSMessage struct {
	Records []S3Event
}

type SQSQueue struct {
	Url  string
	Arn  string
	UUID uuid.UUID
	Name string
}

// A Queue represents an SQS+SNS configuration which will
// allow the user to get messages about newly added  replica objects
type Queue struct {
	SQSClient       sqsiface.SQSAPI
	SNSClient       snsiface.SNSAPI
	Region          string
	AccountId       string
	SNSTopicName    string
	sqsQueue        *SQSQueue
	snsArn          string
	subscriptionArn string
	initialized     bool
}

func (q *Queue) Init() error {
	q.snsArn = fmt.Sprintf("arn:aws:sns:%s:%s:%s", q.Region, q.AccountId, q.SNSTopicName)
	queue, err := q.createQueue()
	if err != nil {
		return fmt.Errorf("failed to create queue: %w", err)
	}
	q.sqsQueue = queue

	err = q.subscribeQueue(queue.Arn)
	if err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}
	q.initialized = true
	return nil
}

func (q *Queue) createQueue() (*SQSQueue, error) {
	queueUUID := uuid.New()
	name := fmt.Sprintf("replica_peer-%s", queueUUID)
	queueArn := fmt.Sprintf("arn:aws:sqs:%s:%s:%s", q.Region, q.AccountId, name)
	policy := fmt.Sprintf(`
{
	"Version": "2012-10-17",
	"Id": "%s/SQSDefaultPolicy",
	"Statement": [
		{
			"Sid": "SNSSend",
			"Effect": "Allow",
			"Principal": {
				"AWS": "*"
			},
			"Action": "SQS:SendMessage",
			"Resource": "%s",
			"Condition": {
				"ArnEquals": {
					"aws:SourceArn": "%s"
				}
			}
		},
		{
			"Sid": "PeerRead",
			"Effect": "Allow",
			"Principal": {
				"AWS": "*"
			},
			"Action": "SQS:ReceiveMessage",
			"Resource": "%s"
		}
	]
}`, queueArn, queueArn, q.snsArn, queueArn)
	createQueueAttributes := map[string]*string{
		"Policy": aws.String(policy),
	}

	newQueue, err := q.SQSClient.CreateQueue(&sqs.CreateQueueInput{
		QueueName:  aws.String(name),
		Attributes: createQueueAttributes,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create queue: %w", err)
	}
	url := newQueue.QueueUrl

	queueAttributeArn := "QueueArn"
	attributes, err := q.SQSClient.GetQueueAttributes(&sqs.GetQueueAttributesInput{
		AttributeNames: []*string{aws.String(queueAttributeArn)},
		QueueUrl:       url,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get queue attributes: %w", err)
	}
	arn := aws.StringValue(attributes.Attributes["QueueArn"])

	return &SQSQueue{
		Url:  *url,
		Name: name,
		UUID: queueUUID,
		Arn:  arn,
	}, nil
}

// DeleteQueue will perform the necessary cleanup tasks to delete the queue
func (q *Queue) DeleteQueue() error {
	_, unsubscribeErr := q.SNSClient.Unsubscribe(&sns.UnsubscribeInput{
		SubscriptionArn: aws.String(q.subscriptionArn),
	})
	queueUrl := q.sqsQueue.Url
	_, deleteQueueErr := q.SQSClient.DeleteQueue(&sqs.DeleteQueueInput{
		QueueUrl: aws.String(queueUrl),
	})
	if unsubscribeErr != nil {
		return fmt.Errorf("failed to unsubscribe: %w", unsubscribeErr)
	}
	if deleteQueueErr != nil {
		return fmt.Errorf("failed to delete queue: %w", deleteQueueErr)
	}
	return nil
}

func (q *Queue) subscribeQueue(arn string) error {
	protocol := "sqs"
	topicArn := fmt.Sprintf("arn:aws:sns:%s:%s:%s", q.Region, q.AccountId, q.SNSTopicName)
	output, err := q.SNSClient.Subscribe(&sns.SubscribeInput{
		Endpoint: aws.String(arn),
		Protocol: aws.String(protocol),
		TopicArn: aws.String(topicArn),
	})
	if err != nil {
		return fmt.Errorf("failed to subscribe to queue: %w", err)
	}
	q.subscriptionArn = aws.StringValue(output.SubscriptionArn)
	return nil
}

// GetMessage will call the handler function for each available message
// If the handler function does not return an error it will delete/acknowledge the mesage otherwise it will not
func (q *Queue) GetMessage(handler func(S3Event) error) error {
	if !q.initialized {
		return fmt.Errorf("queue must be initialized before attempting to GetMessage")
	}
	response, err := q.SQSClient.ReceiveMessage(&sqs.ReceiveMessageInput{
		QueueUrl:        aws.String(q.sqsQueue.Url),
		WaitTimeSeconds: aws.Int64(20),
	})
	if err != nil {
		return fmt.Errorf("failed to receive messages: %w", err)
	}

	for _, message := range response.Messages {
		parsedMessageBody := SQSMessageBody{}
		if err := json.Unmarshal([]byte(aws.StringValue(message.Body)), &parsedMessageBody); err != nil {
			return fmt.Errorf("failed to unmarshal message: %w", err)
		}
		parsedMessage := SQSMessage{}
		if err := json.Unmarshal([]byte(parsedMessageBody.Message), &parsedMessage); err != nil {
			return fmt.Errorf("failed to unmarshal message: %w", err)
		}

		for _, record := range parsedMessage.Records {
			handlerErr := handler(record)
			if handlerErr != nil {
				return fmt.Errorf("failed to handle message: %w", err)
			}
		}

		messageHandle := message.ReceiptHandle
		_, err := q.SQSClient.DeleteMessage(&sqs.DeleteMessageInput{
			QueueUrl:      aws.String(q.sqsQueue.Url),
			ReceiptHandle: messageHandle,
		})
		if err != nil {
			return fmt.Errorf("failed to delete message: %w", err)
		}
	}
	return nil
}
