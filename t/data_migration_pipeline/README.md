# Template: Data migration pipeline

Template for a simple Spanner data migration pipeline in Go, using Apache Beam to migrate from a MySQL CSV dump. 

How to render this template:

1. [Install the abc binary](https://github.com/abcxyz/abc#installation).

1. cd into an empty destination directory.

    ```shell
    $ mkdir ~/spanner_migration_project
    $ cd ~/spanner_migration_project
    ```

1. Render via:

    ```shell
    $ abc templates render github.com/abcxyz/abc.git//t/data_migration_pipeline
    ```

    This will output a file named `main.go` in your working directory containing the template program.

1. Follow the steps in the rendered README.md to run the application.
