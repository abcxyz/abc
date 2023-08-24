# Example Template: REST server

Template for a simple HTTP/JSON REST server implemented in Go, using go-chi for HTTP routing.

How to render this template:

1. [Install the abc binary](https://github.com/abcxyz/abc#installation).

1. cd into an empty destination directory.

    ```shell
    $ mkdir ~/rest_server
    $ cd ~/rest_server
    ```

1. Render via:

    ```shell
    $ abc templates render github.com/abcxyz/abc.git//t/rest_server
    ```

    This will output a file named `main.go` in your working directory containing the transformed program.

1. Follow the steps in the rendered README.md to run the server.
