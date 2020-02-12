# Running

Replica search is a self-bootstrapping, stand-alone HTTP server that can be spun up locally or wherever is convenient for users. It listens on :8080 and receives a query string, and pagination parameters for searches. It indexes and monitors the Replica S3 bucket, and communicates with a [magneticow](https://github.com/boramalper/magnetico/tree/master/cmd/magneticow) instance that wraps a [magneticod](https://github.com/boramalper/magnetico/tree/master/cmd/magneticod) instance that monitors the BitTorrent DHT and indexes that. Replica search handles search requests by combining the results from S3 and BitTorrent.

Run with something like:

    RUST_LOG=main,search=trace RUST_BACKTRACE=1 AWS_PROFILE=replica-searcher cargo run

`AWS_PROFILE` is naming a profile (on my system that's in `~/.aws/credentials` that has the permissions of the AWS `replica-searcher` user. You can view the access keys for this user [here](https://console.aws.amazon.com/iam/home?region=ap-southeast-1#/users/replica-searcher?section=security_credentials). You may need to get a copy of these from an colleague.

`search` is the name of the main module.

The `magneticow` instance to use can be configured with the environment variables `MAGNETICOW_LOCATION`, `MAGNETICO_USER`, and `MAGNETICO_PASSWORD`, and currently defaults to an instance in Paris behind Cloudflare.

# Interface

Searches can be performed by querying [`localhost:8080`]. [Here's](https://github.com/getlantern/replica/blob/dd2a378d03ce4a3d44184ce91c95cd4b97e4c60a/search/src/server.rs#L98-L105) a list of the available query parameters.
