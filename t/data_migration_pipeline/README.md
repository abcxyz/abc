# Template: Data migration pipeline
Simple Spanner data migration pipeline in Go, using Apache Beam to migrate from a MySQL CSV dump.
To run this, please follow these steps:
1. cd into an empty directory

    ```shell
    $ mkdir ~/spanner_migration_project
    $ cd ~/spanner_migration_project
    ```
1. Install the `abc` binary
    ```shell
    $ go install github.com/abcxyz/abc/cmd/abc@latest
    $ abc --help
    ```
    This only works if you have go installed (https://go.dev/doc/install) and have the Go binary directory in your $PATH.
1. Execute the template defined in the `t` directory.
This will output a file named `main.go` in your working directory containing
the template program.
    ```shell
    $ abc templates render github.com/abcxyz/abc.git//t/data_migration_pipeline
    ```
1. Start a local Spanner emulator. If the emulator is not installed already, you will be prompted to download and install the binary for the emulator.
    ```shell
    $ gcloud components update
    $ gcloud emulators spanner start
    ```
1. Create a dedicated gcloud configuration that allows disable authentication and override the endpoint.
Once configured, your gcloud commands will be sent to the emulator instead of the production service. No worries, you'll be able to switch back to your previous configurations at the end of this guide.
    ```shell
    $ gcloud config configurations create emulator
    $ gcloud config set auth/disable_credentials true
    $ gcloud config set project [your-project-id]
    $ gcloud config set api_endpoint_overrides/spanner http://localhost:9020/
    ```
1. Create a test database to host your pipeline output.
    ```shell
    $ gcloud spanner instances create test-instance \
   --config=emulator-config --description="Test Instance" --nodes=1
    $ gcloud spanner databases create testdb --instance=test-instance --ddl='CREATE TABLE mytable (Id STRING(36)) PRIMARY KEY(Id)'
    ```
   - make sure the local Spanner emulator runs in a separated tab.
     
7. Point your client libraries to the emulator.
When pipeline starts, the client library automatically checks for SPANNER_EMULATOR_HOST and connects to the emulator if it is running.
    ```shell
    $ export SPANNER_EMULATOR_HOST=localhost:9010
    ```
1. Run the data migration pipeline.
    ```shell
    $ go run main.go -input-csv-path "test-data.csv" -spanner-database "projects/[your-project-id]/instances/test-instance/databases/testdb" -spanner-table "mytable"
    ```
    - Add `-dry-run=true` to active the dry run mode.

1. Verify the MySQL CSV dump has been successfully migrated to your Spanner database
    ```shell
    $ gcloud spanner databases execute-sql testdb --instance=test-instance --sql='SELECT * FROM mytable'
    ```
1. Switch back to your default gcloud configurations
    ```shell
    $ gcloud config configurations activate default
    ```
