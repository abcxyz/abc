#!/bin/bash
set -eEuo pipefail

function pre_gen_go {
  echo "running pre_gen_go"
  # Create the new directory if it doesn't exist
  if [ ! -d $1 ]; then
    mkdir -p $1
  fi

  # Check if the go.mod exists
  if [ ! -f "$1/go.mod" ]; then
    # Create the file
    cd $1
    go mod init us-go.pkg.dev/my-org/my-repo
    cd -
  fi
}

function post_gen_go {
  echo "running post_gen_go"
  cd $1
  go mod tidy
  cd -
}

function pre_gen_node {
  echo "running pre_gen_node"

  PROTO_VERSION=${PROTO_VERSION:-$(cat VERSION)}

  # Create the new directory if it doesn't exist
  if [ ! -d $1 ]; then
    mkdir -p $1
  fi

  # Configure package manager to use with npm
  cat > $1/.npmrc << EOF
@npm-scope:registry=https://us-npm.pkg.dev/example.com:my-project/my-node-gar/
//us-npm.pkg.dev/example.com:my-project/my-node-gar/:always-auth=true
EOF

  # Create the package.json with appropriate version -- same version can never be published twice
  cat > $1/package.json << EOF
{ 
  "name": "@npm-scope/my-node-gar",
  "version": "$PROTO_VERSION",
  "main": "index.ts",
  "scripts": {
    "artifactregistry-login": "npx google-artifactregistry-auth --repo-config=\".npmrc\" --credential-config=\".npmrc\""
  }
}
EOF
}

function post_gen_node {
  echo "running post_gen_node"
  cd $1
  npm i
  cd -
}

function main {
  pre_gen_go gen/go
  pre_gen_node gen/node

  buf generate

  post_gen_go gen/go
  post_gen_node gen/node
}

main 
