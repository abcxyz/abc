#!/bin/bash

echo -e "I have a working Go program which is an HTTP server that responds with "hello world". I want to create an abc template that outputs this file, but customizes the HTTP response with a user-provided template input. Here's my source code, in the file named example_server.go:\n\n" | cat - example_server.go | go run cmd/abc/abc.go templates llm
