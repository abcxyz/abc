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

### For `abc templates render`

Usage: `abc templates render [flags] <template_location>`

The `<template_location>` parameter is any directory-like location that can be
downloaded by the library that we use for template downloads,
github.com/hashicorp/go-getter. It should contain a `spec.yaml` file in its
root.

Examples of template locations:

- Git repo: `github.com/abcxyz/abc.git`
- A subdirectory of a git repo (useful when a single repo contains multiple
  templates):
  `github.com/abcxyz/abc.git//examples/templates/render/hello_jupiter`
- A specific branch or SHA of a git repo:
  `github.com/abcxyz/abc.git?ref=d3beffc21f324fc23f954c6602c49dfe8f9988e8`
- A local directory (useful for template developers, you can run a template
  without committing and merging it to github):
  `~/git/example.com/myorg/mytemplaterepo`
- A tarball, local or remote: `~/my_downloaded_tarball.tgz`
- A GCS bucket (no example yet)

#### Flags

- `--debug-scratch-contents`: for template authors, not regular users. This will
  print the contents of the scratch directory after executing each step of the
  spec.yaml. Useful for debugging errors like
  `path "src/app.js" doesn't exist in the scratch directory, did you forget to "include" it first?"`
- `--dest <output_dir>`: the directory on the local filesystem to write output
  to. Defaults to the current directory. If it doesn't exist, it will be
  created.
- `--input=key=val`: provide an input parameter to the template. `key` must be
  one of the inputs declared by the template in its `spec.yaml`. May be repeated
  to provide multiple inputs, like
  `--input=name=alice --input=email=alice@example.com`.
- `--log-level`: one of `debug|info|warning|error`. How verbose to log.
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
  directory. Use `--log-level` verbosity of `info` or higher to see the
  locations of the directories.
- `--prompt`: the user will be prompted for inputs that are needed by the
  template but are not supplied by `--inputs`.
- `--skip-input-validation`: don't run any of the validation rules for template
  inputs. This could be useful if a template has overly strict validation logic
  and you know for sure that the value you want to use is OK.

#### Logging

Use the environment variable `ABC_LOG_MODE` to configure JSON logging.

The valid values for `ABC_LOG_MODE` are:

- `dev`: (the default) non-JSON logs, best for human readability in a terminal
- `production`: JSON formatted logs, better for feeding into a program

## User Guide

Start here if you want want to install ("render") a template using this CLI
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
   $ abc templates render \
     github.com/abcxyz/abc.git//examples/templates/render/hello_jupiter
   ```

   Note: this URL format will change to a cleaner format in the future so the
   command will become something like
   `abc templates render abc/examples/templates/render/hello_jupiter`.

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

## Template developer guide

This section explains how you can create a template for others to install (aka
"render").

### Concepts

A template can take many forms. It can be anything downloadable by the
https://github.com/hashicorp/go-getter library, including:

- A GitHub repo
- A .tgz or .zip file downloaded over HTTPS
- A local directory
- A GCP Cloud Storage bucket

In essence, a template is a directory or directory-like object containing a
"spec file", named `spec.yaml`
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
  to the output directory.

Normally, the template and scratch directories are deleted when rendering
completes. For debugging, you can provide the flag `--keep-temp-dirs` to retain
them for inspection.

### The spec file

The spec file, named `spec.yaml` describes the template, including:

- A human-readable description of the template
- What inputs are needed from the user (e.g. their service name)
- The sequence of steps to be executed by the CLI when rendering the template
  (e.g. "replace every instance of `__replace_me_service_name__` with the
  user-provided input named `service_name`).

The following is an example spec file. It has a single templated file,
`main.go`, and during template rendering all instances of the word `world` are
replaced by a user-provided string. Thus "hello, world" is transformed into
"hello, $whatever" in `main.go`.

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

| api_version             | Binary versions | Notes                                           |
| ----------------------- | --------------- | ----------------------------------------------- |
| cli.abcxyz.dev/v1alpha1 | From 0.0.0      | Initial version                                 |
| cli.abcxyz.dev/v1beta1  | From 0.2        | Adds support for an `if` predicate on each step |

#### Template inputs

Typically the CLI user will supply certain values as `--input=inputname=value`
which will be used by the spec file (such as `whomever` in the preceding
example). Alternatively, the user can use `--prompt` rather than `--input` to
enter values interactively.

A template may not need any inputs, in which case the `inputs` top-level field
can be omitted.

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
action: 'action-name' # One of 'include', 'print', 'append', 'string_replace', 'regex_replace', `regex_name_lookup`, `go_template
if: 'bool(my_input) || int(my_other_input) > 42' # Optional CEL expression
params:
  foo: bar # The params differ depending on the action
```

### Action: `include`

Copies files or directories from the template directory to the scratch
directory.

Params:

- `paths`: a list of files and/or directories to copy. These may use template
  expressions (e.g. `{{.my_input}}`). Directories will be crawled recursively
  and every file underneath will be processed. By default, the output location
  of each file is the same as its location in the template directory.
- `as`: as list of output locations relative to the output directory. This can
  be used to make the output location(s) different than the input locations. If
  `as` is present, its length must be equal to the length of `paths`; that is,
  each path must be given an output location.

  These may use template expressions (e.g. `{{.my_input}}`).

- `skip`: omits some files or directories that might be present in the input
  paths. For each path in `paths`, if `$path/$skip` exists, it won't be included
  in the output. This supports use cases like "I want every thing in this
  directory except this specific subdirectory and this specific file."

  It's not an error if the path to skip wasn't found.

  As a special case, the template spec file is automatically skipped and omitted
  from the template output when the template has an `include` for the path `.`.
  We assume that when template authors say "copy everything in the template into
  the output," they mean "everything except the spec file."

  These may use template expressions (e.g. `{{.my_input}}`).

- `from`: rarely used. The only currently valid value is `'destination'`. This
  copies files into the scratch from the _destination_ directory instead of the
  _template_ directory. The `paths` must point to files that exist in the
  destination directory (which defaults to the current working directory. See
  the example below.

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

### Action: `print`

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

  - `{{._flag_dest}}`: the value of the the `--dest` flag, e.g. `.`
  - `{{._flag_source}}`: the template location that's being rendered, e.g.
    `github.com/abcxyz/abc.git//t/my_template`

Example:

```yaml
- action: 'print'
  params:
    message:
      'Please go to the GCP console for project {{.project_id}} and click the
      thing'
```

### Action: `append`

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

### Action: `string_replace`

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

### Action: `regex_replace`

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

### Action: `regex_name_lookup`

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

### Action: `go_template`

Executes a file as a Go template, replacing the file with the template output.

Params:

- `paths`: A list of files and/or directories in which to do the replacement.
  May use template expressions (e.g. `{{.my_input}}`). Directories will be
  crawled recursively and every file underneath will be processed. These files
  will be rendered with Go's
  [text/template templating language](https://pkg.go.dev/text/template).

#### Example:

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

### Action: `for_each`

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
