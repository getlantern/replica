# Running

Run with something like:

    RUST_LOG=search=trace RUST_BACKTRACE=1 AWS_PROFILE=replica-searcher cargo build

`AWS_PROFILE` is naming a profile (on my system that's in `~/.aws/credentials` that has the permissions of the AWS `replica-searcher` user. `search` is the name of the main module.

If the implementation is using the Rocket dependency (it probably isn't anymore) you will require `nightly` rust. From the `search` directory run `rustup override set nightly`.

# Interface

Searches can be performed by querying `localhost:8000` with a search query "s", for example `http://localhost:8000?s=something`.
