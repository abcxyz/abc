# abc

**abc is not an official Google product.**

## Introduction

abc is a command line interface (CLI) to speed up the process of creating new
applications. It achieves this by using a templating system that will allow
users to interactively fork existing templates, while providing neccessary
context, instructions, or requested inputs.

Using this tool will reduce the cognitive load required to set up GitHub actions
properly, or follow development best practices, and avoid copy/pasting from
various sources to start a new project.

This doc contains a [User Guide](#user-guide) and a
[Template Developer Guide](#template-developer-guide).

## Command line usage

The `abc` command has many subcommands, describes below. In abc versions before
0.9, these commands were called `abc templates $SUBCOMMAND`, but as of 0.9 they
are now also available under the shorter form `abc $SUBCOMMAND`.

### For `abc render`

Usage: `abc render [flags] <template_location>`

Example:
`abc render --prompt github.com/abcxyz/gcp-org-terraform-template@latest`

The `<template_location>` parameter is one of these two things:

- A remote git repository. The subdirectory is optional, defaulting to the root
  of the repo. This directory must contain a `spec.yaml`. The version suffix
  must be either `@latest`, long commit SHA, branch name or tag. Short commit
  SHA's are not supported and if provided, they will be tried as a branch or tag
  name. Examples:

  - `github.com/abcxyz/gcp-org-terraform-template@latest` (no subdirectory)
  - `github.com/abcxyz/abc/t/rest_server@latest` (with subdirectory)
  - `github.com/abcxyz/abc/t/rest_server@v0.2.1` (uses tag instead of "latest")
  - `github.com/abcxyz/abc/t/rest_server@main` (use branch name instead of
    "latest")
  - `github.com/abcxyz/abc/t/rest_server@0402ed8413f02e1069c2aec368eca208895918b1`
    (use ref to long commit SHA)

- A local directory as an absolute or relative path. This directory must contain
  a `spec.yaml`. Examples:
  - `/my/template/dir`
  - `my/template/dir`
  - `./my/template/dir` (equivalent to previous)

#### Flags

- `--dest <output_dir>`: the directory on the local filesystem to write output
  to. Defaults to the current directory. If it doesn't exist, it will be
  created.
- `--input=key=val`: provide an input parameter to the template. `key` must be
  one of the inputs declared by the template in its `spec.yaml`. May be repeated
  to provide multiple inputs, like
  `--input=name=alice --input=email=alice@example.com`. Every input must
  correspond to a template input value (unless you also provide
  `--ignore-unknown-inputs`.
- `--input-file=file`: provide a YAML file with input(s) to the template. The
  file must contain a YAML object whose keys and values are strings. If a key
  exists in the file but is also provided as an `--input`, the `--input` value
  takes precedence.

  This flag may be repeated, like
  `--input-file=some-inputs.yaml --input-file=more-inputs.yaml`. When there are
  multiple input files, they must not have any overlapping keys.

  Any inputs in this file that are not accepted by the template are ignored.

- `--git-protocol=[https|ssh]`: controls the protocol to use when connecting to
  a remote git repository. The default is to use https, but you may want to use
  ssh if you want to authenticate using SSH keys. You can also set the
  environment variable `ABC_GIT_PROTOCOL=ssh` if you don't want to type this
  flag for every abc command.
- `--force-overwrite`: normally, the template rendering operation will abort if
  the template would output a file at a location that already exists on the
  filesystem. This flag allows it to continue.
- `--keep-temp-dirs`: there are two temp directories created during template
  rendering. Normally, they are removed at the end of the template rendering
  operation, but this flag causes them to be kept. Inspecting the temp
  directories can be helpful in debugging problems with `spec.yaml` or with the
  `abc` command itself. The two temp directories are the "template directory",
  into which the template is downloaded, and the "scratch directory", where
  files are staged during transformations before being written to the output
  directory. Use environment variable `ABC_LOG_LEVEL=debug` to see the locations
  of the directories.
- `--prompt`: the user will be prompted for inputs that are needed by the
  template but are not supplied by `--inputs` or `--input-file`. You can specify
  the environment variable `ABC_PROMPT=true` to avoid typing this every time.
- `--skip-input-validation`: don't run any of the validation rules for template
  inputs. This could be useful if a template has overly strict validation logic
  and you know for sure that the value you want to use is OK.
- `--ignore-unknown-inputs`: silently ignore any `--input` values that aren't
  accepted by the template.

Flags for template developers:

- `--debug-step-diffs`: for template authors, not regular users. This will log
  the diffs made by each step as git commits in a tmp git repository. If you
  want to see the git logs and diffs with your usual git commands, please
  navigate to the tmp folder, otherwise you will need to use a git flag
  `--git-dir=path/to/tmp/debug/folder` for your commands, e.g.:
  `git --git-dir=path/to/tmp/debug/folder log`. A warn log will show you where
  the tmp repository is.

  Note: you must have git installed to use this flag.

- `--debug-scratch-contents`: for template authors, not regular users. This will
  print the filename of every file in the scratch directory after executing each
  step of the spec.yaml. Useful for debugging errors like
  `path "src/app.js" doesn't exist in the scratch directory, did you forget to "include" it first?"`.


#### Logging

Use the environment variables `ABC_LOG_MODE` and `ABC_LOG_LEVEL` to configure
logging.

The valid values for `ABC_LOG_MODE` are:

- `text`: (the default) non-JSON logs, best for human readability in a terminal
- `json`: JSON formatted logs, better for feeding into a program

The valid values for `ABC_LOG_LEVEL` are `debug`, `info`, `notice`, `warning`,
`error`, and `emergency`. The default is `warn`.

### For `abc golden-test`

The golden-test feature is essentially unit testing for templates. You provide
(1) a set of template input values and (2) the expected output directory
contents. The test framework verifies that the actual output matches the
expected output, using the `verify` subcommand. Separately, the `record`
subcommand helps with capturing the current template output and saving it as the
"expected" output for future test runs. This concept is similar to "snapshot
testing" and
"[rpc replay testing](https://pkg.go.dev/cloud.google.com/go/rpcreplay)." In
addition, the `new-test` subcommand creates a new golden test to initialize the
needed golden test directory structure and `test.yaml`.

Each test is configured by placing a file named `test.yaml` in a subdirectory of
the template named `testdata/golden/<your-test-name>`. See below for details on
this file.

Usage:

- `abc golden-test new-test [options] <test_name> [<location>]`
  see `abc golden-test new-test --help` for supported options.
- `abc golden-test record [--test-name=<test_name>] [<location>]`
- `abc golden-test verify [--test-name=<test_name>] [<location>]`

Note: For `new-test`, the `<location>` parameter gives the location of the template.
For `record` and `verify`, `<location>` parameter gives the location that include one or more templates and abc cli
will recursively search for templates and tests under the given `<location>`. 

Examples:

- `abc golden-test new-test basic examples/templates/render/hello_jupiter`
  creates a new golden-test for the specific named test called `basic` for the
  given template
- `abc golden-test verify examples/templates/render/hello_jupiter`
  runs all golden-tests for the given template
- `abc golden-test verify --test-name=example_test examples/templates/render/hello_jupiter`
  same as above, but only for the specific named tests
- `abc golden-test verify examples/templates`
  runs all golden-tests for the templates included in `examples/templates`
- `abc golden-test verify --test-name=example examples/templates`
  same as above, but only for the specific named tests
- `abc golden-test verify`
  runs all golden-tests for the templates included in the current directory
- `abc golden-test record examples/templates/render/hello_jupiter`
  record the current template output as the desired/expected output for all
  tests within the given template, saving to `testdata/golden/<test_name>/data`.
- `abc golden-test record --test-name=one_env,multiple_envs examples/templates/render/for_each_dynamic`
  same as above, but only for the specific named tests.
- `abc golden-test record examples/templates`
  record the all template outputs as the desired/expected outputs for all test cases for the templates under `example/templates` directory.
- `abc golden-test record --test-name=example examples/templates`
  same as above, but only for the specific named tests.
- `abc golden-test record`
  record the all template outputs as the desired/expected outputs for all test cases for the templates under current directory.

For `record` and `verify` subcommand, the `<test_name>` parameter gives the test names to record or verify, if not
specified, all tests will be run against. This flag may be repeated, like
`--test-name=test1`, `--test-name=test2`, or `--test-name=test1,test2`.

For `new-test` subcommand, the `<location>` parameter gives the location of the template, defaults to the current directory.

For `record` and `verify` subcommand, the `<location>` parameter gives the location that include one or more templates, defaults to the current directory.

For every test case, it is expected that a
`testdata/golden/<test_name>/test.yaml` exists to define template input params.
Each "input" in this file must correspond to a template input defined in the
template's `spec.yaml`. Each required input in the template's spec.yaml must
have a corresponding input value defined in the `test.yaml`. Typically, you will use
`golden-test new-test` subcommand to initialize the 
needed golden test directory structure and `test.yaml`,
but it's also possible to create the 
desired goldentest directory structure and `test.yaml` by hand.

Example test.yaml:

```yaml
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'GoldenTest'

inputs:
  - name: 'my-service-account'
    value: 'platform-ops@abcxyz-my-project.iam.gserviceaccount.com'
  - name: 'my-project-number'
    value: '123456789'
```

The expected/desired test output for each test is stored in
`testdata/golden/<test_name>/data`. Typically, you'll use the
`golden-test record` subcommand to populate this directory, but it's also
possible to create the desired output files by hand.

#### Builtin vars in golden tests

In `spec.yaml`, there some [built-in variables](#built-in-template-variables)
like `_git_tag` that are populated automatically based on the environment. For
unit testing, we need the ability to populate these variables with fixed values
so the test output is the same every time.

To support this, the `test.yaml` file may have a top-level field `builtin_vars`
that sets the value of built-in variables while running the test. For example:

```yaml
api_version: 'cli.abcxyz.dev/v1beta2'
kind: 'GoldenTest'

inputs:
  - name: 'some-normal-input'
    value: 'some-value'

# For the purposes of this golden test, provide a fake _git_tag value.
builtin_vars:
  - name: '_git_tag'
    value: 'my-cool-tag'
```

Technicalities:

- Any built-in variables that are _not_ set by `builtin_vars` will not be in
  scope. For example, if your spec.yaml references `{{._git_tag}}`, but
  test.yaml doesn't provide a value for `_git_tag` using `builtin_vars` in
  `test.yaml`, then the golden-test command will fail with an error about an
  unknown variable.
- You can't set an arbitrary variable name; only a specific known set of
  variable names are allowed (e.g. `_git_sha`, `_git_tag`, `_flag_dest`).
- Built-in variable names always start with underscore.

### For `abc describe`

The describe command downloads the template and prints out its description, and
describes the inputs that it accepts.

Usage:

- `abc describe <template_location>`

The `<template_location>` takes the same value as the
[render](#for-abc-render) command.

Example:

Command:

```
abc describe github.com/abcxyz/guardian/abc.templates/default-workflows@v0.1.0-alpha12
```

Output:

```
Description:  Generate the Guardian workflows for the Google Cloud organization Terraform intrastructure repo.

Input name:   terraform_directory
Description:  A sub-directory for all Terraform files
Default:      .

Input name:   terraform_version
Description:  The terraform version to use with Guardian
Default:      1.5.4

Input name:   guardian_wif_provider
Description:  The Google Cloud workload identity federation provider for Guardian

Input name:   guardian_service_account
Description:  The Google Cloud service account for Guardian
Rule 0:       gcp_matches_service_account(guardian_service_account)

Input name:   guardian_state_bucket
Description:  The Google Cloud storage bucket for Guardian state
```

## User Guide

Start here if you want to install ("render") a template using this CLI
tool. "Rendering" a template is when you use the `abc` CLI to download some
template code, do some substitution to replace parts of it with your own values,
and write the result to a local directory.

## Installation

There are two ways to install:

1.  The most official way:

    - Go to https://github.com/abcxyz/abc/releases
    - Pick the most recent release that isn't an `-alpha` or `-rc` or anything,
      just `vX.Y.Z`
    - Download the `.tar.gz` file that matches your OS and CPU:

      - Linux: `linux_amd64`
      - Mac:
        - M1/M2/later: `darwin_arm64`
        - Intel: `darwin_amd64`

      You can use the `curl -sSL` command to download. Please substitute the
      version number you're downloading:

          $ curl -sSL https://github.com/abcxyz/abc/releases/download/v1.2.3/abc_1.2.3_linux_amd64.tar.gz | tar -xzv abc

    - Now you will have an `abc` file that you can run. Perhaps place it in your
      `$PATH`.

2.  Alternatively, if you already have a Go programming environment set up, just
    run `go install github.com/abcxyz/abc/cmd/abc@latest`.

### Tab Autocompletion

Optionally, for tab autocompletion, run:

`COMP_INSTALL=1 COMP_YES=1 abc`

This will add a `complete` command to your .bashrc or corresponding file.

## Rendering a template

The full user journey looks as follows. For this example, suppose you want to
create a "hello world" Go web service.

1. Set up a directory that will receive the rendered template output, and `cd`
   to it.
   - Option A: for local experimentation, you can just write into any directory,
     for example: `mkdir ~/template_experiment && cd ~/template_experiment`
   - Option B: to create a real service that you'll share with others:
     - create a new git repo that will contain your new service
     - clone it onto your machine (`git clone ...`)
     - `cd` into the git directory you just cloned into
     - Create a branch (`git checkout -b template_render`)
   - Option C: if you know what you're doing, you can create a local repo using
     `git init` and worry later about connecting it to an upstream repo.
2. Find the template to install. We assume that you already know the URL of a
   template that you want to install by reading docs or through word-of-mouth.
   There is a best-effort list of known templates in
   [template-index.md](template-index.md). For this example, suppose we're
   installing the "hello jupiter" example from the abc repo.
3. Run the `render` command:

   ```shell
   $ abc render \
     github.com/abcxyz/abc/examples/templates/render/hello_jupiter@latest
   ```

   This command will output files in your curent directory that are the result
   of executing the template.

   - (Optional) examine the resulting files and try running the code:

     ```shell
     $ ls
     main.go
     $ go run main.go
     Hello, jupiter!
     ```

4. Git commit, push your branch, create a PR, get it reviewed, and submit it.

   ```shell
   $ git add -A
   $ git commit -am 'Initial output of template rendering'
   $ git push origin template_render:$USER/template_render

   # Assuming you're using GitHub, now go create a PR.
   ```

### Authentication errors

If `abc` asks you for a username and password, that probably means that the
template you're rendering is in a private git repository, and HTTPS
authentication didn't work. You may want to try cloning over SSH instead. To use
SSH, you can add `--git-protocol=ssh` to your command line or set the
environment variable `ABC_GIT_PROTOCOL=ssh`.

## Template developer guide

This section explains how you can create a template for others to install (aka
"render").

### Concepts

A template is installed from a location that you provide. These locations may be
either a GitHub repository or a local directory. If you install a template from
GitHub, it will be downloaded into a temp directory by `abc`.

In essence, a template is a directory containing a "spec file", named
`spec.yaml`
([example](https://github.com/abcxyz/abc/blob/main/examples/templates/render/hello_jupiter/spec.yaml)),
and other files such as source code and config files.

### Model of operation

Template rendering has a few phases:

- The template is downloaded and unpacked into a temp directory, called the
  "template directory."
- The spec.yaml file is loaded and parsed as YAML from the template directory
- Another temp directory called the "scratch directory" is created.
- The steps in the spec.yaml file are executed in sequence:
  - `include` actions copy files and directories from the template directory to
    the scratch directory. This is analogous to a Dockerfile COPY command. For
    example:
    ```yaml
    - action: 'include'
      params:
        paths: ['main.go']
    ```
  - The `append`, `string_replace`, `regex_replace`, `regex_name_lookup`, and
    `go_template` actions transform the files that are in the scratch directory
    at the time they're executed.
    - This means that for example a string_replace after an append will affect
      the appended text, but if put before it will not.
- Once all steps are executed, the contents of the scratch directory are copied
  to the `--dest` directory (which default to your current working directory).

Normally, the template and scratch directories are deleted when rendering
completes. For debugging, you can provide the flag `--keep-temp-dirs` to retain
them for inspection.

### The spec file

The spec file, named `spec.yaml` describes the template, including:

- A human-readable description of the template
- The version of the YAML schema that is used by this file; we may add or remove
  fields from spec.yaml
- What inputs are needed from the user (e.g. their GCP service account name or
  the port number to listen on)
- The sequence of steps to be executed by the CLI when rendering the template
  (e.g. "replace every instance of `__replace_me_service_account__` with the
  user-provided input named `service_account`).

Here is an example spec file. It has a single templated file, `main.go`, and
during template rendering all instances of the word `world` are replaced by a
user-provided string. Thus "hello, world" is transformed into "hello, $whatever"
in `main.go`.

```yaml
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'Template'

desc:
  'An example template that changes a "hello world" program to a "hello whoever"
  program'
inputs:
  - name: 'whomever'
    desc: 'The name of the person or thing to say hello to'
steps:
  - desc: 'Include some files and directories'
    action: 'include'
    params:
      paths: ['main.go']
  - desc: 'Replace "world" with user-provided input'
    action: 'string_replace'
    params:
      paths: ['main.go']
      replacements:
        - to_replace: 'world'
          with: '{{.whomever}}'
```

#### List of api_versions

The `api_version` field controls the interpretation of the YAML file. Some
features are only available in more recent versions.

The currently valid versions are:

| api_version             | Supported in abc CLI versions | Notes                                                                         |
| ----------------------- | ----------------------------- | ----------------------------------------------------------------------------- |
| cli.abcxyz.dev/v1alpha1 | 0.0.0 and up                  | Initial version                                                               |
| cli.abcxyz.dev/v1beta1  | 0.2.0 and up                  | Adds support for an `if` predicate on each step in spec.yaml                  |
| cli.abcxyz.dev/v1beta2  | 0.4.0 and up                  | Adds: <br>- the top-level `ignore` field in spec.yaml<br>- Path globs         |
| cli.abcxyz.dev/v1beta3  | 0.5.0                         | Adds: <br>- `_git_*` builtin variables                                        |
| cli.abcxyz.dev/v1beta4  | 0.6.0                         | Adds: <br>- independent rules                                                 |
| cli.abcxyz.dev/v1beta5  | 0.6.0                         | Same as v1beta4 for [complex reasons](https://github.com/abcxyz/abc/pull/431) |
| cli.abcxyz.dev/v1beta6  | 0.7.0                         | Adds: the `_now_ms` variable and `formatTime` function in Go-templates        |

#### Template inputs

Typically the CLI user will supply certain values as `--input=inputname=value`
which will be used by the spec file (such as `whomever` in the preceding
example). Alternatively, the user can use `--prompt` rather than `--input` to
enter values interactively.

A template may not need any inputs, in which case the `inputs` top-level field
in the spec.yaml can be omitted.

Each input in the `inputs` list has these fields:

- `name`: an identifier for this input that will be used in template expressions
  (like `{{.myinput}}`) and CEL expressions.
- `description`: documentation for the users of your template to help them
  understand what value to enter for this input.
- `default` (optional): the string value that will be used if the user doesn't
  supply this input. If an input doesn't have a default, then a value for that
  input must be given by the CLI user.
- `rules`: a list of validation rule objects. Each rule object has these fields:

  - `rule`: a CEL expression that returns true if the input is valid.

    This CEL expression has access to all each input value as a CEL variable of
    the same name (see examples below). The type in CEL of each input variable
    is always `string`. You can convert a string of digits to a number using
    `int(my_input)` if you need to do numeric comparisons; see the
    "min_size_bytes" example below.

    This CEL expression can call extra CEL functions that we added to address
    common validation needs [link](#using-cel), such as
    `gcp_matches_project_id(string)` and `gcp_matches_service_account(string)`.

  - `message` (optional): a message to show to the CLI user if validation fails.
    The template author can use this to tell the user what input format is
    valid.

The input validation `rules` may be skipped with the `--skip-input-validation`
flag, documented above.

An example input without a default:

```yaml
inputs:
  - name: 'output_filename'
    desc: 'The name of the file to create'
```

An example input _with_ a default:

```yaml
inputs:
  - name: 'output_filename'
    description: 'The name of the file to create'
    default: 'out.txt'
```

An example of parsing an input as an integer:

```yaml
inputs:
  - name: 'disk_size_bytes'
    rules:
      - rule: 'int(disk_size_bytes)' # Will fail if disk_size_bytes (which is a string) can't be parsed as int
        message: 'Must be an integer'
```

An example input with a validation rule:

```yaml
inputs:
  - name: 'project_id_to_use'
    rules:
      - rule: 'gcp_matches_project_id(project_id_to_use)'
        message: 'Must be a GCP project ID'
```

An example of validating multiple inputs together:

```yaml
inputs:
  - name: 'min_size_bytes'
  - name: 'max_size_bytes'
    rules:
      - rule: 'int(min_size_bytes) <= int(max_size_bytes)'
        message: "the max can't be less than the min"
```

##### Top-level rules

Most of the time, validation rules will be part of an `inputs` declaration as
described above. But it's also possible to create validation rules that are
independent of any input and go at the topmost scope of the spec file.

To use this feature, your spec.yaml must declare
`api_version: cli.abcxyz.dev/v1beta4` or greater.

Example:

```yaml
apiVersion: "cli.abcxyz.dev/v1beta4"
kind: "Template"
desc: "An example of using independent rules"
rules:
  - rule: '_git_sha != ""'
    message: "this template must be installed from a git repo"
```

#### Built-in template variables

Besides the template inputs described above, there are built-in template
variables that are automatically provided. These can be referenced in a
go-template context as `{{._my_variable}}` and in a CEL context as
`my_variable`. The built-in variables are:

- `_git_sha`: If the template source is a git repo (local or remote), then this
  will be set to the hex git SHA of the source. If the template is NOT being
  rendered from git (for example, it's being rendered from a local directory
  that's not in a git repo), then this variable will exist but its value will be
  empty string.

  Available in `api_version`s v1beta3 and later.

  The motivating use case for this is to allow terraform module sources to be
  pinned to the same SHA of the template that is being rendered.

  Example:

  ```
  steps:
    - desc: 'Replace git_sha_goes_here with actual git sha in terraform files'
      action: 'string_replace'
      params:
        paths: ['*.tf']
        replacements:
          - to_replace: 'git_sha_goes_here'
            with: '{{._git_sha}}'
  ```

- `_git_short_sha`: It's like `_git_sha` above, except it's only the first 7
  characters rather than the full SHA.

  Available in `api_version`s v1beta3 and later.

- `_git_tag`: If...

  1. the template source is a git repo (local or remote)
  1. there is a tag corresponding to the template source SHA

  ... then `_git_tag` will be set to the git tag name. Otherwise `_git_tag` will
  be an empty string.

  There's one rare edge case: if the template source SHA has _multiple_ tags
  that point to it, we break the tie as follows. We attempt to parse the tags as
  semantic version and take the latest one. If that doesn't work because the
  tags are not semver, then we take the lexicographically largest tag name.

  Available in `api_version`s v1beta3 and later.

- `_flag_dest`: this variable is only in scope within the `params` field of a
  `print` action. It contains the destination directory that the template is
  being rendered to. It's intended to be used to show instructions to the user,
  like `message: "cd into {{._flag_dest}} and run the foo command`.

- `_flag_source`: this variable is only in scope within the `params` field of a
  `print` action. It contains the source directory that the template is being
  rendered from.

#### Templating

Most fields in the spec file can use template expressions that reference the
input values. In the above example, the replacement value of `{{.whomever}}`
means "the user-provided input value named `whomever`." This uses the
[text/template templating language](https://pkg.go.dev/text/template) that is
part of the Go standard library.

### Steps and actions

Each step of the spec file performs a single action. A single step consists of:

- an optional string named `desc`
- a required string named `action`
- (in `api_version` >= v1beta1) an optional string named `if` containing CEL
  predicate (more [below](#using-cel) on CEL).
- a required object named `params` whose fields depend on the `action`

Example:

```yaml
desc: 'An optional human-readable description of what this step is for'
action: 'action-name' # One of 'include', 'print', 'append', 'string_replace', 'regex_replace', `regex_name_lookup`, `go_template`, `for_each`
if: 'bool(my_input) || int(my_other_input) > 42' # Optional CEL expression
params:
  foo: bar # The params differ depending on the action
```

#### Action: `include`

Copies files or directories from the template directory to the scratch
directory. It's similar to the `COPY` command in a Dockerfile.

Params:

- `paths`: a list of files and/or directories to copy. These may use template
  expressions or file globs (e.g. `{{.my_input}}`, `*.txt`). Directories will be
  crawled recursively and every file underneath will be processed. By default,
  the output location of each file is the same as its location in the template
  directory.
- `as`: as list of output locations relative to the output directory. This can
  be used to make the output location(s) different than the input locations. If
  `as` is present, its length must be equal to the length of `paths`; that is,
  each path must be given an output location. These may use template expressions
  (e.g. `{{.my_input}}`). If file globs are used in `paths`, the corresponding
  `as` inputs will be treated as directories:

  ```yaml
  - paths: ['*.txt', '*.md', 'nonglob.json']
    as: ['dir1', 'dir2', 'newname.json']
  ```

  ```
  output:

  /dir1/file1.txt
  /dir1/file2.txt
  /dir2/file3.md
  /dir2/file4.md
  /newname.json
  ```

- `skip`: omits some files or directories that might be present in the input
  paths. For each path in `paths`, if `$path/$skip` exists, it won't be included
  in the output. This supports use cases like "I want every thing in this
  directory except this specific subdirectory and this specific file."

  It's not an error if the path to skip wasn't found.

  As a special case, the template spec file is automatically skipped and omitted
  from the template output when the template has an `include` for the path `.`.
  We assume that when template authors say "copy everything in the template into
  the output," they mean "everything except the spec file."

  These may use template expressions or file globs (e.g. `{{.my_input}}`,
  `*.txt`).

- `from`: rarely used. The only currently valid value is `'destination'`. This
  allows the template to modify a file that is already present on the user's
  filesystem. This copies files into the scratch from the _destination_
  directory instead of the _template_ directory. The `paths` must point to files
  that exist in the destination directory (which defaults to the current working
  directory. See the example below.

Examples:

- A simple include, where each file keeps it location:

  ```yaml
  - action: 'include'
    params:
      paths: ['main.go', '{{.user_requested_config}}/config.txt']
  ```

- Using `as` to relocate files:

  ```yaml
  - action: 'include'
    params:
      paths: ['{{.dbname}}/db.go']
      as: ['db.go']
  ```

- Using `skip` to omit certain sub-paths:

  ```yaml
  - action: 'include'
    params:
      paths: ['configs']
      skip: ['unwanted_subdir', 'unwanted_file.txt']
  ```

- Appending to a file that already exists in the destination directory using
  `from: destination`:

  ```yaml
  - action: 'include'
    params:
      from: 'destination'
      paths: ['existing_file_in_dest_dir.txt']
  - action: 'append'
    params:
      paths: ['existing_file_in_dest_dir.txt']
      with: "I'm a new line at the end of the file"
  ```

#### Action: `print`

Prints a message to standard output. This can be used to suggest actions to the
user.

Params:

- `message`: the message to show. May use template expressions (e.g.
  `{{.my_input}}`).

  The print action has special access to extra template variables named
  `_flag_*` containing the values of _some_ of the command line flags. This can
  be useful to print a message like
  `Template rendering is done, now please to go the {{._flag_dest}} directory and run a certain command.`
  The available values are:

  - `{{._flag_dest}}`: the value of the `--dest` flag, e.g. `.`
  - `{{._flag_source}}`: the template location that's being rendered, e.g.
    `github.com/abcxyz/abc/t/my_template@latest`

Example:

```yaml
- action: 'print'
  params:
    message:
      'Please go to the GCP console for project {{.project_id}} and click the
      thing'
```

#### Action: `append`

Appends a string on the end of a given file. File must already exist. If no
newline at end of `with` parameter, one will be added unless
`skip_ensure_newline` is set to `true`.

If you need to remove an existing trailing newline before appending, use
`regex_replace` instead.

Params:

- `paths`: List of files and/or directory trees to append to end of. May use
  template expressions (e.g. `{{.my_input}}`). Directories will be crawled
  recursively and every file underneath will be processed.
- `with`: String to append to the file.
- `skip_ensure_newline`: Bool (default false). When true, a `with` not ending in
  a newline will result in a file with no terminating newline. If `false`, a
  newline will be added automatically if not provided.

Example:

```yaml
- action: 'append'
  params:
    paths: ['foo.html', 'web/']
    with: '</html>\n'
    skip_ensure_newline: false
```

#### Action: `string_replace`

Within a given list of files and/or directories, replaces all occurrences of a
given string with a given replacement string.

Params:

- `paths`: a list of files and/or directories in which to do the replacement.
  May use template expressions (e.g. `{{.my_input}}`). Directories will be
  crawled recursively and every file underneath will be processed.
- `replacements`: a list of objects, each having the form:
  - `to_replace`: the string to search for. May use template expressions (e.g.
    `{{.my_input}}`).
  - `with`: the string to replace with. May use template expressions (e.g.
    `{{.my_input}}`).

Example:

```yaml
- action: 'string_replace'
  params:
    paths: ['main.go']
    replacements:
      - to_replace: 'Alice'
        with: '{{.sender_name}}'
      - to_replace: 'Bob'
        with: '{{.receiver_name}}'
```

#### Action: `regex_replace`

Within a given list of files and/or directories, replace a regular expression
(or a subgroup thereof) with a given string.

Params:

- `paths`: A list of files and/or directories in which to do the replacement.
  May use template expressions (e.g. `{{.my_input}}`). Directories will be
  crawled recursively and every file underneath will be processed.
- `replacements`: a list of objects, each having the form:

  - `regex`: an
    [RE2 regular expression](https://github.com/google/re2/wiki/Syntax),
    optionally containing named subgroups (like `(?P<mygroupname>[a-z]+)`. May
    use template expressions.

    Non-named subgroups (like `(abc)|(def)`)are not supported, for the sake of
    readability. Use a non-capturing group (like `(?:abc)|(?:abc)`) if you need
    grouping without capturing.

    Note that by default, RE2 doesn't use multiline mode, so ^ and $ will match
    the start and end of the entire file, rather than each line. To enter
    multiline mode you need to set the flag by including this:
    `(?m:YOUR_REGEX_HERE)`. More information available in RE2 docs.

  - `with`: a string to that will replace regex matches (or, if the
    `subgroup_to_replace` field is set, will replace only that subgroup). May
    use template expressions and may use
    [Regexp.Expand() syntax](https://pkg.go.dev/regexp#Regexp.Expand) (e.g.
    `${mysubgroup}`).

    Regex expansion (e.g. `${mygroup}`) happens before _before_ go-template
    expansion (e.g. `{{ .myinput }}`; that means you can use a subgroup to name
    an input variable, like `{{ .${mygroup} }}`. That expression means "the
    replacement value is calculated by taking the text of the regex subgroup
    named `mygroup` and looking up the user-provided input variable having that
    name." This is covered in the examples below.

Examples:

- Find `gcp_project_id=(anything)` and replace `x` with the user-provided input
  named `project_id`:

  ```yaml
  - action: 'regex_replace'
    params:
      paths: ['main.go']
      replacements:
        - regex: 'gcp_project_id=[a-z0-9-]+'
          with: 'gcp_project_id={{.project_id}}'
  ```

- Do the same thing as above, in a different way:

  ```yaml
  - action: 'regex_replace'
    params:
      paths: ['main.go']
      replacements:
        - regex: 'gcp_project_id=(?P<proj_id>[a-z0-9-]+)'
          subgroup_to_replace: 'proj_id'
          with: '{{.project_id}}'
  ```

- Even more fancy: replace all instances of `gcp_$foo=$bar` with
  `gcp_$foo=$user_provided_input_named_foo`:

  ```yaml
  - action: 'regex_replace'
    params:
      paths: ['main.go']
      replacements:
        - regex: 'gcp_(?P<input_name>[a-z_]+)=(?P<value>[a-z0-9-]+)'
          subgroup_to_replace: 'value'
          with: '{{ .${input_name} }}'
  ```

- Replace all instances of `template_me_$foo=$bar` with
  `$foo=$user_provided_input_named_foo`:

  ```yaml
  - action: 'regex_replace'
    params:
      paths: ['main.go']
      replacements:
        - regex: 'template_me_(?P<input_name>[a-z_]+)=(?P<value>[a-z0-9-]+)'
          with: '${input_name}={{ .${input_name} }}'
  ```

#### Action: `regex_name_lookup`

`regex_name_lookup` is similar to `regex_replace`, but simpler to use, at the
cost of generality. It matches a regular expression and replaces each named
subgroup with the input variable whose name matches the subgroup name.

Params:

- `paths`: A list of files and/or directories in which to do the replacement.
  May use template expressions (e.g. `{{.my_input}}`). Directories will be
  crawled recursively and every file underneath will be processed.
- `replacements`: a list of objects, each having the form:
  - `regex`: an
    [RE2 regular expression](https://github.com/google/re2/wiki/Syntax)
    containing one or more named subgroups. Each subgroup will be replaced by
    looking up the input variable having the same name as the subgroup.

Example: replace all appearances of `template_me` with the input variable named
`myinput`:

```yaml
- action: 'regex_name_lookup'
  params:
    paths: ['main.go']
    replacements:
      - regex: '(?P<myinput>template_me)'
```

#### Action: `go_template`

Executes a file as a Go template, replacing the file with the template output.

Params:

- `paths`: A list of files and/or directories in which to do the replacement.
  May use template expressions (e.g. `{{.my_input}}`). Directories will be
  crawled recursively and every file underneath will be processed. These files
  will be rendered with Go's
  [text/template templating language](https://pkg.go.dev/text/template).

Example:

Suppose you have a file named `hello.html` that looks like this, with a
`{{.foo}}` template expression:

```
<html><body>
{{ if .friendly }}
Hello, {{.person_name}}!
{{ else }}
Go jump in a lake, {{.person_name}}.
{{ end }}
</body></html>
```

This action will replace `{{.person_name}}` (and all other template expressions)
with the corresponding inputs:

```yaml
- action: 'go_template'
  params:
    paths: ['hello.html']
```

#### Action: `for_each`

The `for_each` action lets you execute a sequence of steps repeatedly for each
element of a list. For example, you might want your template to create several
copies of a given file, one per application environment (e.g. production,
staging).

There are two variants of `for_each`. One variant accepts a hardcoded YAML list
of values to iterate over in the `values` field. The other variant accepts a CEL
expression in the `values_from` field that outputs a list of strings.

Variant 1 example: hardcoded list of YAML values:

```yaml
- desc: 'Iterate over each (hard-coded) environment'
  action: 'for_each'
  params:
    iterator:
      key: 'environment'
      values: ['production', 'dev']
    steps:
      - desc: 'Do some action for each environment'
        action: 'print'
        params:
          message: 'Now processing environment named {{.environment}}'
```

Variant 2 example: a CEL expression that produces the list to iterate over:

```yaml
- desc: 'Iterate over each environment, produced by CEL as a list'
  action: 'for_each'
  params:
    iterator:
      key: 'environment'
      values_from: 'comma_separated_environments.split(",")'
    steps:
      - desc: 'Do some action for each environment'
        action: 'print'
        params:
          message: 'Now processing environment named {{.environment}}'
```

Params:

- `iterator`: an object containing the key `key`, and exactly one of `values` or
  `values_from`.
  - `key`: the name of the index variable that assumes the value of each element
    of the list.
  - `values`: a list of strings to iterate over.
  - `values_from`: a CEL expression that outputs a list of strings.
- `steps`: a list of steps/actions to execute in the scope of the for_each loop.
  It's analogous to the `steps` field at the top level of the spec file.

### Ignore (Optional)

This `ignore` feature is similiar to `skip` in `include` action, the difference
here is that ignore is global and it applies to every `include` action.

We use [filepath Match](https://pkg.go.dev/path/filepath#Match) to match the
file and directory paths that should be ignored if included/copied to
destination directory. In addition, we also match file and directory names using
the same accepted patterns.

This section is optional, if not provided, a default ignore list is used:
`.DS_Store`, `.bin`, and `.ssh`, meaning all file and directory matching these
names will be ignored. To set your custom ignore list, please check accepted
patterns [here](https://pkg.go.dev/path/filepath#Match). Note: a leading slash
in a pattern here means the source of the included paths.

Example:

```yaml
ignore:
  # Ignore `.ssh` under root, root is the template dir or destination dir if the
  # included paths are from destination.
  - '/.ssh'
  # Ignore all txt files with name `tmp.txt` recursively (under root and its
  # sub-directories).
  - 'tmp.txt'
  # Ignore all txt files in the sub-directories with folder depth of 2.
  - '*/*.txt'
  # Ignore all cfg files recursively.
  - '*.cfg'
steps:
  - desc: 'Include some files and directories'
    action: 'include'
    params:
      paths: ['.ssh', 'src_dir']
  - desc: 'Include some files and directories from destination'
    action: 'include'
    params:
      paths: ['dest_dir']
      from: 'destination'
```

### Post-rendering validation test (golden test)

We use post-rendering validation tests to record (capture the anticipated
outcome akin to expected output in unit test) and subsequently verify template
rendering results.

To add golden tests to your template, all you need is to create a
`testdata/golden` folder under your template, and a
`testdata/golden/<test_name>/test.yaml` for each of your tests to define test
metadata and input parameters.

The test.yaml for a post-rendering validation test may look like,

```yaml
api_version: 'cli.abcxyz.dev/v1alpha1'
kind: 'GoldenTest'

inputs:
  - name: 'input_a'
    value: 'a'
  - name: 'input_b'
    value: 'b'
```

Then you can use `abc golden-test` to record (capture the anticipated
outcome akin to expected output in unit test)or verify the tests.

# Using CEL

We use the CEL language to allow template authors to embed scripts in the spec
file in certain places. The places you can use CEL are:

- the `from_values` field inside `for_each` that produces a list of values to
  iterate over
- the `rule` field inside an `input` that validates the input and returns a
  boolean
- (starting in `api_version` v1beta1) the `if` field inside a
  [step](#steps-and-actions) object

[CEL, the Common Expression Language)](https://github.com/google/cel-spec), is a
non-Turing complete language that's designed to be easily embedded in programs.
"Expression" means "a computation that produces a value", like `1+1` or
`["shark"+"nado", "croco"+"gator"]`.

The CEL expressions you write in your spec file will have access to the template
inputs, as in this example:

For example:

```yaml
- desc: 'Iterate over each environment, produced by CEL as a list'
  action: 'for_each'
  params:
    iterator:
      key: 'env'
      values: 'input.comma_separated_environments.split(",")'
```

The above example also shows the `split` function, which is not part of the core
CEL language. It's a "custom function" that we added to CEL to support a common
need for templates (see [below](#custom-functions-reference)).

## Custom functions reference

These are the functions that we added that are not normally part of CEL.

- `gcp_matches_project_id(string)` returns whether the input matches the format
  of a GCP project ID.

  You might want to use this for a template that creates a project or references
  an existing project.

  Examples:

      gcp_matches_project_id("my-project") == true
      gcp_matches_project_id("example.com:my-project") == true

- `gcp_matches_service_account(string)` returns whether the input matches a full
  GCP service account name. It can be either an API-created service account or a
  platform-created service agent.

  You might want to use this for a template that requires a reference to an
  already-created service account.

  Example:

      gcp_matches_service_account("platform-ops@abcxyz-my-project.iam.gserviceaccount.com") == true
      gcp_matches_service_account("platform-ops") == false

- `gcp_matches_service_account_id(string)` returns whether the input matches the
  part of a GCP service account name before the "@" sign.

  You might want to use this for a template that creates a service account.

  Example:

      gcp_matches_service_account_id("platform-ops") == true
      gcp_matches_service_account_id("platform-ops@abcxyz-my-project.iam.gserviceaccount.com") == false

- `matches_capitalized_bool(string)`: returns whether the input is a stringified
  boolean starting with a capitalized letter, as used in Python.

  This function doesn't accept boolean inputs because the whole point is that
  we're checking the string form of a boolean for its capiltalization.

  Examples:

      matches_capitalized_bool("True") == true
      matches_capitalized_bool("False") == true
      matches_capitalized_bool("true") == false
      matches_capitalized_bool("false") == false
      matches_uncapitalized_bool("something_else") == false

- `matches_uncapitalized_bool(string)`: returns whether the input is a
  stringified boolean starting with a capitalized letter, as used in Go,
  Terraform, and others.

  This function doesn't accept boolean inputs because the whole point is that
  we're checking the string form of a boolean for its capiltalization.

  Example expressions:

      matches_uncapitalized_bool("true") == true
      matches_uncapitalized_bool("false") == true
      matches_uncapitalized_bool("True") == false
      matches_uncapitalized_bool("False") == false
      matches_uncapitalized_bool("something_else") == false

- `string.split(split_char)`: we added a "split" method on strings. This has the
  same semantics as Go's
  [strings.Split function](https://pkg.go.dev/strings#Split).

  Example:

      "abc,def".split(",") == ["abc", "def"]

## Update Checks
[abcxyz/abc-updater](https://github.com/abcxyz/abc-updater) is run once a day to
check for newer versions of `abc`, results are printed to stderr if an update is
available. This check can be disabled by setting the environment variable
`ABC_IGNORE_VERSIONS=ALL`. Notifications can be disabled for specific versions
with a list of versions and constraints `ABC_IGNORE_VERSIONS=<2.0.0,3.5.0`.

This check is not done on non-release builds, as they don't have canonical
version to check against.
