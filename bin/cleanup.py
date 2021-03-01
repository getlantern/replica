#!/usr/bin/env python3

# This contains stuff relating to cleaning up Replica S3 stuff. Currently
# that's just removing subscriptions that have no endpoints (Amazon says
# somewhere they're supposed to clean these up for you?). You should clean up
# the SQS queues first.

import boto3
import logging

sqs = boto3.client("sqs")
sns = boto3.client("sns")


def all_subscriptions():
    client = sns
    kwargs = dict(
        TopicArn="arn:aws:sns:ap-southeast-1:670960738222:replica-search-events"
    )
    while True:
        response = client.list_subscriptions_by_topic(**kwargs)
        for sub in response["Subscriptions"]:
            yield sub
        if "NextToken" not in response:
            break
        kwargs["NextToken"] = response["NextToken"]


def arn_exists(arn):
    _, _, service, _, _, name = arn.split(":")
    if service != "sqs":
        raise ValueError("unexpected service", service)
    try:
        sqs.get_queue_url(QueueName=name)
    except sqs.exceptions.QueueDoesNotExist:
        return False
    return True


def remove_orphaned_subs():
    for sub in all_subscriptions():
        if arn_exists(sub["Endpoint"]):
            logging.info("leaving subscription for %r", sub["Endpoint"])
            continue
        logging.info("deleting subscription for %r", sub["Endpoint"])
        sns.unsubscribe(SubscriptionArn=sub["SubscriptionArn"])


def main():
    logging.basicConfig(level=logging.DEBUG)
    logging.getLogger("botocore").setLevel(logging.INFO)
    logging.getLogger("urllib3").setLevel(logging.INFO)
    remove_orphaned_subs()


if __name__ == "__main__":
    main()
