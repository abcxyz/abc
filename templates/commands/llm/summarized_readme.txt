**abc: A Command Line Interface for Template-Based Application Development**

**Introduction**

abc is a command line interface (CLI) that simplifies the process of creating new applications by utilizing a templating system. This system allows users to interactively fork existing templates while providing necessary context, instructions, or requested inputs. By leveraging abc, users can reduce the cognitive load required to set up GitHub actions and follow development best practices, eliminating the need for copy/pasting from various sources.

**Command Line Usage**

**For `abc templates render`**

* Usage: `abc templates render [flags] <template_location>`
* Example: `abc templates render --prompt github.com/abcxyz/gcp-org-terraform-template@latest`

The `<template_location>` parameter can be either a remote git repository or a local directory containing a `spec.yaml` file.

**Flags**

* `--debug-step-diffs`: For template authors, not regular users. Logs the diffs made by each step as git commits in a temporary git repository.
* `--debug-scratch-contents`: For template authors, not regular users. Prints the filename of every file in the scratch directory after executing each step of the spec.yaml.
* `--dest <output_dir>`: The directory on the local filesystem to write output to. Defaults to the current directory.
* `--input=key=val`: Provide an input parameter to the template. `key` must be one of the inputs declared by the template in its `spec.yaml`. May be repeated to provide multiple inputs.
* `--input-file=file`: Provide a YAML file with input(s) to the template. The file must contain a YAML object whose keys and values are strings. If a key exists in the file but is also provided as an `--input`, the `--input` value takes precedence.
* `--force-overwrite`: Normally, the template rendering operation will abort if the template would output a file at a location that already exists on the filesystem. This flag allows it to continue.
* `--keep-temp-dirs`: There are two temp directories created during template rendering. Normally, they are removed at the end of the template rendering operation, but this flag causes them to be kept. Inspecting the temp directories can be helpful in debugging problems with `spec.yaml` or with the `abc` command itself.
* `--prompt`: The user will be prompted for inputs that are needed by the template but are not supplied by `--inputs` or `--input-file`.
* `--skip-input-validation`: Don't run any of the validation rules for template inputs. This could be useful if a template has overly strict validation logic and you know for sure that the value you want to use is OK.

**Logging**

Use the environment variables `ABC_LOG_MODE` and `ABC_LOG_LEVEL` to configure logging.

**For `abc templates golden-test`**

The golden-test feature is essentially unit testing for templates. You provide (1) a set of template input values and (2) the expected output directory contents. The test framework verifies that the actual output matches the expected output, using the `verify` subcommand. Separately, the `record` subcommand helps with capturing the current template output and saving it as the "expected" output for future test runs. This concept is similar to "snapshot testing" and "[rpc replay testing](https://pkg.go.dev/cloud.google.com/go/rpcreplay)." In addition, the `new-test` subcommand creates a new golden test to initialize the needed golden test directory structure and `test.yaml`.

Each test is configured by placing a file named `test.yaml` in a subdirectory of the template named `testdata/golden/<your-test-name>`. See below for details on this file.

**Usage**

* `abc templates golden-test new-test [options] <test_name> [<location>]`
* `abc templates golden-test record [--test-name=<test_name>] [<location>]`
* `abc templates golden-test verify [--test-name=<test_name>] [<location>]`

**For `abc templates describe`**

The describe command downloads the template and prints out its description, and describes the inputs that it accepts.

**Usage**

* `abc templates describe <template_location>`

**User Guide**

**Installation**

There are two ways to install:

1. **Official Method:**
   * Visit https://github.com/abcxyz/abc/releases
   * Download the `.tar.gz` file that matches your OS and CPU
   * Run the `abc` file

2. **Go Environment:**
   * Run `go install github.com/abcxyz/abc/cmd/abc@latest`

**Rendering a Template**

1. Create a directory to receive the rendered template output.
2. Find the template to install.
3. Run the `render` command:

   ```shell
   $ abc templates render \
     github.com/abcxyz/abc/examples/templates/render/hello_jupiter@latest
   ```

**Template Developer Guide**

**Concepts**

A template is a directory containing a "spec file" (`spec.yaml`) and other files such as source code and config files.

**Model of Operation**

Template rendering has a few phases:

* The template is downloaded and unpacked into a temp directory.
* The spec.yaml file is loaded and parsed.
* Another temp directory called the "scratch directory" is created.
* The steps in the spec.yaml file are executed in sequence:
    * `include` actions copy files and directories from the template directory to the scratch directory.
    * `append`, `string_replace`, `regex_replace`, `regex_name_lookup`, and `go_template` actions transform the files in the scratch directory.
* Once all steps are executed, the contents of the scratch directory are copied to the `--dest` directory (which defaults to your current working directory).

**The spec file**

The spec file describes the template, including:

* A description
* The version of the YAML schema that is used
* What inputs are needed from the user
* The sequence of steps to be executed by the CLI when rendering the template

**Template Inputs**

Template inputs are typically supplied by the CLI user as `--input=inputname=value`. Alternatively, the user can use `--prompt` to enter values interactively.

**Steps and Actions**

Each step of the spec file performs a single action. A single step consists of:

* An optional description
* A required action
* An optional CEL predicate
* A required object containing parameters that depend on the action

**Action: `include`**

Copies files or directories from the template directory to the scratch directory.

**Action: `print`**

Prints a message to standard output.

**Action: `append`**

Appends a string on the end of a given file.

**Action: `string_replace`**

Within a given list of files and/or directories, replaces all occurrences of a given string with a given replacement string.

**Action: `regex_replace`**

Within a given list of files and/or directories, replace a regular expression (or a subgroup thereof) with a given string.

**Action: `regex_name_lookup`**

Similar to `regex_replace`, but simpler to use, at the cost of generality. It matches a regular expression and replaces each named subgroup with the input variable whose name matches the subgroup name.

**Action: `go_template`**

Executes a file as a Go template, replacing the file with the template output.

**Action: `for_each`**

Executes a sequence of steps repeatedly for each element of a list.

**Using CEL**

CEL (Common Expression Language) is used to allow template authors to embed scripts in the spec file. CEL expressions have access to the template inputs.

**Custom Functions Reference**

* `gcp_matches_project_id(string)`: Returns whether the input matches the format of a GCP project ID.
* `gcp_matches_service_account(string)`: Returns whether the input matches a full GCP service account name.
* `gcp_matches_service_account_id(string)`: Returns whether the input matches the part of a GCP service account name before the "@" sign.
* `matches_capitalized_bool(string)`: Returns whether the input is a stringified boolean starting with a capitalized letter, as used in Python.
* `matches_uncapitalized_bool(string)`: Returns whether the input is a stringified boolean starting with a capitalized letter, as used in Go, Terraform, and others.
* `string.split(split_char)`: A "split" method on strings with the same semantics as Go's `strings.Split` function.
