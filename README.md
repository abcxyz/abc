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

## User Guide

Start here if you want want to install ("render") a template using this CLI
tool. "Rendering" a template is when you use the `abc` CLI to download some
template code, do some substitution to replace parts of it with your own values,
and write the result to a local directory.

## One-time CLI installation

The quick answer is to just run
`go install github.com/abcxyz/abc/cmd/abc@latest` .

This only works if you have `go` installed (https://go.dev/doc/install) and have
the Go binary directory in your `$PATH` (try `PATH=$PATH:~/go/bin`).

This is the temporary installation process until we start formally releasing
precompiled binaries.

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
2. Find the template to install. There's currently no central database of
   templates that exist. We assume that you already know the URL of a template
   that you want to install by reading docs or through word-of-mouth. For this
   example, suppose we're installing the "hello jupiter" example from the abc
   repo.
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

It's possible for multiple templates to live in the same git repo, directory,
tar file, etc. When this happens, there are multiple spec files, and the CLI
user provides the `--spec=foo.yaml` flag to choose which spec file to execute.
The `--spec` path is relative to the template root that was provided by the CLI.
For example, if the user ran
`abc templates render --spec=foo.yaml github.com/abcxyz/abc.git//examples/templates/render/hello_jupiter`,
then the `foo.yaml` should be in the `hello_jupiter` directory.

TODO document flags

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
"spec file", usually named `spec.yaml`
([example](https://github.com/abcxyz/abc/blob/main/examples/templates/render/hello_jupiter/spec.yaml)),
and other files such as source code and config files.

### Model of operation

Template rendering has a few phases:

- The template is downloaded and unpacked into a temp directory, called the
  "template directory."
- The spec file is loaded and parsed as YAML from the template directory
- Another temp directory called the "scratch directory" is created.
- The steps in the spec file are executed in sequence:
  - `include` actions copy files and directories from the template directory to
    the scratch directory. This is analogous to a Dockerfile COPY command. For
    example:
    ```yaml
    - action: 'include'
      params:
        paths: ['main.go']
    ```
  - The `string_replace`, `regex_replace`, `regex_name_lookup`, and
    `go_template` actions transform the files that are in the scratch directory
    at the time they're executed.
- Once all steps are executed, the contents of the scratch directory are copied
  to the output directory.

Normally, the template and scratch directories are deleted when rendering
completes. For debugging, you can provide the flag `--keep-temp-dirs` to retain
them for inspection.

### The spec file

The spec file describes the template, including:

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
apiVersion: 'cli.abcxyz.dev/v1alpha1'
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

#### Templating

Most fields in the spec file can use template expressions that reference the
input values. In the above example, the replacement value of `{{.whomever}}`
means "the user-provided input value named `whomever`." This uses the
[text/template templating language](https://pkg.go.dev/text/template) that is
part of the Go standard library.

### Steps and actions

Each step of the spec file performs a single action. A single step consists of
an optional `desc`, a string `action`, and a `params` object whose fields depend
on the `action`:

```yaml
desc: 'An optional human-readable description of what this step is for'
action: 'action-name' # One of 'include', 'print', 'string_replace', 'regex_replace', `regex_name_lookup`, `go_template
params:
  foo: bar # The params differ depending on the action
```

### Action: `include`

Copies files or directories from the template directory to the scratch
directory.

Params:

- `paths`: a list of files and/or directories to copy. These may use template
  expressions (e.g. `{{.my_input}}`). By default, the output location of each
  file is the same as its location in the template directory.
- `as`: as list of output locations relative to the output directory. This can
  be used to make the output location(s) different than the input locations. If
  `as` is present, its length must be equal to the length of `paths`; that is,
  each path must be given an output location.

  `as` may not be used with `strip_prefix` or `add_prefix`.

  These may use template expressions (e.g. `{{.my_input}}`).

- `strip_prefix`: computes the output path by stripping off the beginning of the
  input path. Useful for relocating files to a different location in the output
  than their input location in the template.

  If `strip_prefix` is not actually a prefix of every element of `paths`, that's
  an error.

  `strip_prefix` may be used with `add_prefix`. `strip_prefix` is executed
  before `add_prefix` if both are present.

  `strip_prefix` can't be used with `as`.

  These may use template expressions (e.g. `{{.my_input}}`).

- `add_prefix`: computes the output path by prepending to the beginning of the
  input path. Useful for relocating files to a different location in the output
  than their input location in the template.

  `add_prefix` may be used with `strip_prefix`. `strip_prefix` is executed
  before `add_prefix` if both are present.

  `add_prefix` can't be used with `as`.

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

- Using `strip_prefix` and `add_prefix` to relocate files:

  ```yaml
  - action: 'include'
    params:
      paths: ['my/config/files']
      strip_prefix: 'my/config/files'
      add_prefix: 'config_files'
  ```

- Using `skip` to omit certain sub-paths:

  ```yaml
  - action: 'include'
    params:
      paths: ['configs']
      skip: ['unwanted_subdir', 'unwanted_file.txt']
  ```

### Action: `print`

Prints a message to standard output. This can be used to suggest actions to the
user.

Params:

- `message`: the message to show. May use template expressions (e.g.
  `{{.my_input}}`).

Example:

```yaml
- action: 'print'
  params:
    message:
      'Please go to the GCP console for project {{.project_id}} and click on the
      thing'
```

### Action: `string_replace`

Within a given list of files and/or directories, replaces all occurrences of a
given string with a given replacement string.

Params:

- `paths`: a list of files and/or directories in which to do the replacement.
  May use template expressions (e.g. `{{.my_input}}`).
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
  May use template expressions (e.g. `{{.my_input}}`).
- `replacements`: a list of objects, each having the form:

  - `regex`: an
    [RE2 regular expression](https://github.com/google/re2/wiki/Syntax),
    optionally containing named subgroups (like `(?P<mygroupname>[a-z]+)`. May
    use template expressions.

    Non-named subgroups (like `(abc)|(def)`)are not supported, for the sake of
    readability. Use a non-capturing group (like `(?:abc)|(?:abc)`) if you need
    grouping without capturing.

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
  May use template expressions (e.g. `{{.my_input}}`).
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
  May use template expressions (e.g. `{{.my_input}}`). These files will be
  rendered with Go's
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
