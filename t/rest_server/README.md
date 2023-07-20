# Example: REST server

Simple HTTP/JSON REST server implemented in Go, using go-chi for HTTP routing.

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

    This only works if you have go installed (https://go.dev/doc/install) and have the Go binary directory in your $PATH (try PATH=$PATH:~/go/bin).

1. Execute the template defined in the `t` directory.
This will output a file named `main.go` in your working directory containing
the transformed program.

    ```shell
    $ abc templates render github.com/abcxyz/abc.git/t/rest_server
    ```

1. Run the transformed program:

    ```shell
    $ go run .
    [yyyy/mm/dd hh:mm:ss] starting server on 8080
    ```

1. In a separate shell, run:

    ```shell
    $ curl localhost:8080
    {"message":"hello world"}
    ```
