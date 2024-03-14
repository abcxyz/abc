#!/bin/bash

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
    go mod init github.com/REPLACE_GITHUB_ORG_NAME/REPLACE_GITHUB_REPO_NAME
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

  proto_version_key='proto_version:'
  proto_version_file='version.yaml'

  PROTO_VERSION=${PROTO_VERSION:-$(grep $proto_version_key $proto_version_file | cut -c $((${#proto_version_key} + 1))- | xargs)}  

  # Create the new directory if it doesn't exist
  if [ ! -d $1 ]; then
    mkdir -p $1
  fi

  # Configure package manager to use with npm
  cat > $1/.npmrc << EOF
@REPLACE_NPM_SCOPE:registry=https://us-npm.pkg.dev/REPLACE_PROJECT_ID/REPLACE_NODE_GAR_REPOSITORY_NAME/
//us-npm.pkg.dev/REPLACE_PROJECT_ID/REPLACE_NODE_GAR_REPOSITORY_NAME/:always-auth=true
EOF

  # Create the package.json with appropriate version -- same version can never be published twice
  cat > $1/package.json << EOF
{ 
  "name": "@REPLACE_NPM_SCOPE/REPLACE_NODE_GAR_REPOSITORY_NAME",
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
  if [ $? -ne 0 ]; then
    echo "Failed buf generate"
    exit 1
  fi

  post_gen_go gen/go
  post_gen_node gen/node
}

main 
