# Simple REST server

This is a simple HTTP/JSON REST server implemented in Go, using go-chi for HTTP routing.

How to run this server:

1. Run the server:

    ```shell
    $ go run .
    [yyyy/mm/dd hh:mm:ss] starting server on 8080
    ```
1. In a separate shell, run:
    ```shell
    $ curl localhost:8080
    {"message":"hello world"}
    ```
