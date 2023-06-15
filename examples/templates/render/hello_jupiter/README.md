# Example: Hello, jupiter

This is the simplest possible demonstration of how templating works. Here we
just take a "hello world" program and turn it into a "hello jupiter" program
by doing a simple string replacement.

To run this, cd to the root of this git repo, then run these steps:

1. First, compile the `abc` binary:

    ```
    $ go build
    ```

2. Now execute the template defined by the .spec file in the example directory.
This will output a file named `main.go` in your working directory containing
the transformed program.

    ```
    $ ./abc templates render examples/templates/render/hello_jupiter
    ```
    
1. Now run the transformed program:

    ```
    $ go run main.go
    Hello, jupiter!
    ```
