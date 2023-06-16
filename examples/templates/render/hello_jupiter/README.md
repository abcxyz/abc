# Example: Hello, jupiter

This is the simplest possible demonstration of how templating works. Here we
just take a "hello world" program and turn it into a "hello jupiter" program
by doing a simple string replacement.

To run this, cd to the root of this git repo, then run these steps:

1. Compile the `abc` binary:

    ```shell
    $ go build
    ```

1. (Optional) Install shell completions:

    ```sh
    COMP_INSTALL=1 COMP_YES=1 ./abc
    ```

1. Execute the template defined by the `spec.yaml` file in the example directory.
This will output a file named `main.go` in your working directory containing
the transformed program.

    ```shell
    $ ./abc templates render examples/templates/render/hello_jupiter
    ```

1. Run the transformed program:

    ```shell
    $ go run main.go
    Hello, jupiter!
    ```
