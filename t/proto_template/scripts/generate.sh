#!/usr/bin/env bash
set -eEuo pipefail

# Runs language specific protoc commands
# $1 parameter which represents the destination folder i.e. gen/go
# $2 parameter which represents proto file i.e. hello.proto
function execute_protoc() {
  dest_root_dir=$1
  proto_file=$2
  lang=$(basename "$dest_root_dir")
  case $lang in
    go)
      export PATH="$PATH:$(go env GOPATH)/bin"
      if ! protoc --go_out=$dest_root_dir $proto_file 
      then
          echo "protoc not recognized, check the README on how to install"
          exit 1
      fi      
  esac
}

# Recursively loop through the current directory and its subdirectories
# $1 parameter which represents the folder where the raw protos are stored i.e. protos
function generate_protos_rec() {
  for file_or_dir in "$1"/*; do
    trimmed_path=${file_or_dir#$raw_protos_dir/}
    if [ -f "$file_or_dir" ]; then
      # run protoc
      execute_protoc $dest_root_dir $file_or_dir
    elif [ -d "$file_or_dir" ]; then
      generate_protos_rec "$file_or_dir"
    fi
  done
}

# Checks if the go folder exists, and optionally initializes a go module if necessary
# $1 parameter which represents the folder location for go protos i.e. gen/go
function pre_gen_go {
  # Create the new directory if it doesn't exist
  if [ ! -d $1 ]; then
    mkdir -p $1
  fi

  # Check if the go.mod exists
  if [ ! -f "$1/go.mod" ]; then
    cd $1
    if ! go mod init github.com/REPLACE_GITHUB_ORG_NAME/REPLACE_GITHUB_REPO_NAME
    then
        echo "go mod init not recognized, add go to your PATH"
        exit 1
    fi
    cd -
  fi
}

# Runs go mod tidy to ensure dependencies are cleaned up
# $1 parameter which represents the folder location for go protos i.e. gen/go
function post_gen_go() {
  cd $1
  go mod tidy
  cd -
}

# main function responsible for generating language specific protos
function main() {
  # go protos
  dest_root_dir=gen/go
  pre_gen_go $dest_root_dir
  generate_protos_rec $raw_protos_dir
  post_gen_go $dest_root_dir
}

# Set the protos directory
raw_protos_dir=protos

main 
