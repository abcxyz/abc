# Context

You must have various tools installed in order to generate protos. The tool does not distinguish between language types and instead seeks to generate protos for a variety of supported languages. Currently only go is supported. Node will be supported in the near future.

## Tools

Install `protoc`, a compiler for generating language specific protos based on .proto files. 

```
  brew install protobuf # for MacOS users
  apt install -y protobuf-compiler # for Linux users
```

### Go

Ensure you have go installed, if not run the following

```
  brew install go
  go version # confirm your go installation is recent i.e. 1.21
```

Must have `protoc-gen-go` installed.

```
  go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
```

# Setup

1. Create an empty repository within your organization dedicated to protos. 

2. Navigate to the root of your repository.

3. Then run the following command.

    ```
    abc templates render \
      --input=github_org_name=<your org name>  \
      --input=github_repo_name=<your repo name> \
      https://github.com/abcxyz/abc/tree/main/t/proto_template
    ```

4. You should now have a copy of the `protos` and `scripts` directories.

5. Run the following commands to execute the script

    ```
    chmod +x ./scripts/generate.sh
    ./scripts/generate.sh
    ```

6. You should now have `gen` folder with all the language specific generated proto files.

# Local Development
Local development assumes the Setup section has already been executed. To iterate on protos, make code changes to existing proto files or create new directories to organize by service. To generate your protos run the generate script as mentioned in the Setup section.

