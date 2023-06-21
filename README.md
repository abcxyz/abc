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

## User Guide

---

Start here if you want want to install ("render") a template using this CLI
tool. "Rendering" a template is when you use the `abc` CLI to download some
template code, do some substitution to replace parts of it with your own values,
and write the result to a local directory.

The full user journey looks is as follows. For this example, suppose you want to
create a "hello world" Go web service.

## One-time CLI installation

The quick answer is to just run
`go install github.com/abcxyz/abc/cmd/abc@latest` .

This only works if you have `go` installed (https://go.dev/doc/install) and have
the Go binary directory in your `$PATH` (try `PATH=$PATH:~/go/bin`).

This is the temporary installation process until we start formally releasing
precompiled binaries.

## Rendering a template

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
