# Template: Data migration pipeline

Simple local data migration pipeline in Go, using Apache Beam.

To run this, cd to the root of this git repo, then run these steps:

1. cd into an empty directory

    ```shell
    $ mkdir ~/template_tmp
    $ cd ~/template_tmp
    ```
1. Install the `abc` binary
    ```shell
    $ go install github.com/abcxyz/abc/cmd/abc@latest
    $ abc --help
    ```
    This only works if you have go installed (https://go.dev/doc/install) and have the Go binary directory in your $PATH.
