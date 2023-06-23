# Example: REST server

Simple HTTP/JSON REST server implemented in Go, using go-chi for HTTP routing.

To run this, cd to the root of this git repo, then run these steps:

1. Compile the `abc` binary:

    ```shell
    $ go build
    ```

1. Execute the template defined by the `spec.yaml` file in the example directory.
This will output a file named `main.go` in your working directory containing
the transformed program.

    ```shell
    $ ./abc templates render examples/templates/render/rest_server
    ```

1. Run the transformed program:

    ```shell
    $ go run main.go
    yyyy/mm/dd hh:mm:ss starting server on :8080
    ```

1. In a separate shell, run:

    ```shell
    $ curl localhost:8080
    {"message":"hello world"}
    ```