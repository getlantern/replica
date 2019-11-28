# Running

The Rocket dependency requires `nightly` rust. From this directory run `rustup override set nightly`. After doing this, you can run with something like:

    RUST_LOG=search=trace RUST_BACKTRACE=1 AWS_PROFILE=replica-searcher cargo build

`AWS_PROFILE` is naming a profile (on my system that's in `~/.aws/credentials` that has the permissions of the AWS `replica-searcher` user. `search` is the name of the main module.

# Interface

Searches can be performed by querying `localhost:8000` with one or more `term` query keys, as in `http://localhost:8000?term=jpg&term=