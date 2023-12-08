// Copyright 2023 The Authors (see AUTHORS file)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package flags contains flags that are commonly used by several commands.
package flags

type CommonFlags struct {
	// Positional arguments:

	// Source is the location of the input template to be rendered.
	//
	// Example: github.com/abcxyz/abc/t/rest_server@latest
	Source string

	// GitProtocol is either https or ssh.
	GitProtocol string
}
