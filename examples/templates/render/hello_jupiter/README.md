# Example: Hello, jupiter

This is the simplest possible demonstration of how templating works. Here we
just take a "hello world" program and turn it into a "hello jupiter" program by
doing a simple string replacement.

To run this:

1.  cd into an empty directory

    ```shell
    $ mkdir ~/template_tmp
    $ cd ~/template_tmp
    ```

1.  Install the `abc` binary

    ```shell
    $ go install github.com/abcxyz/abc/cmd/abc@latest
    $ abc --help
    ```

    This only works if you have go installed (https://go.dev/doc/install) and
    have the Go binary directory in your $PATH (try PATH=$PATH:~/go/bin).

1.  Execute the template defined by the `spec.yaml` file in the example
    directory. This will output a file named `main.go` in your working directory
    containing the transformed program.

        ```shell
        $ abc render github.com/abcxyz/abc/examples/templates/render/hello_jupiter@latest
        ```

1.  Run the transformed program:

    ```shell
    $ go run .
    Hello, jupiter!
    ```
