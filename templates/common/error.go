// Copyright 2024 The Authors (see AUTHORS file)
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

package common

import "fmt"

// An implementation of error that contains an command exit status. This is
// intended to be returned from a Run() function when a command wants to
// return a specific error code to the OS.
type ExitCodeError struct {
	Code int
	Err  error
}

func (e *ExitCodeError) Error() string {
	// The CLI user should never see this, it's unwrapped in main().
	return fmt.Sprintf("exit code %d: %v", e.Code, e.Err)
}

func (e *ExitCodeError) Unwrap() error {
	return e.Err
}
