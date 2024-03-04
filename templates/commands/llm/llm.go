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

package llm

import (
	"context"
	_ "embed"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"cloud.google.com/go/vertexai/genai"
	"github.com/abcxyz/pkg/cli"
)

//go:embed prompt.txt
var partialPrompt []byte

//gp:embed summarized_readme.txt
var summarizedReadme []byte

type Command struct {
	cli.BaseCommand
}

// Desc implements cli.Command.
func (c *Command) Desc() string {
	return "instantiate a template to setup a new app or add config files"
}

// Help implements cli.Command.
func (c *Command) Help() string {
	return "TODO"
}

func (c *Command) Flags() *cli.FlagSet {
	set := c.NewFlagSet()
	return set
}

func (c *Command) Run(ctx context.Context, args []string) error {
	const (
		projectID = "revell-dev-3b7cbc"
		location  = "us-central1"
		modelName = "gemini-pro"
	)

	userRequest, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	client, err := genai.NewClient(ctx, projectID, location)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	model := client.GenerativeModel(modelName)

	model.GenerationConfig = genai.GenerationConfig{
		Temperature: ptr[float32](0.05),
	}
	cs := model.StartChat()

	send := func(msg string) *genai.GenerateContentResponse {
		// fmt.Printf("== Me: %s\n== Model:\n", msg)
		res, err := cs.SendMessage(ctx, genai.Text(msg))
		if err != nil {
			log.Fatal(err)
		}
		return res
	}

	prompt := ""
	prompt += "Here's the summary of the README.md documentation file for abc. This was summarized by an LLM specifically for you:\n\n```\n"
	prompt += string(summarizedReadme)
	prompt += "\n```\nThat's the end of the summary of the README.md documentation file.\n\n"

	prompt += string(partialPrompt)
	prompt += `Now let's generate a template. The following paragraph is a request from a user. You should output a spec.yaml, possibly along with instructions for transforming their existing files with placeholder values to be replaced by template actions. You may ask for clarification if anything is unclear. Please create a spec.yaml file that accomplishes their goal and explain how it works. When the user talks about "output", they probably mean "outputting a file from the template using the include action", and not "print a message using the print action". Remember to include instructions for adding placeholder values into the user's input files, if needed. For example, if the instructions are to replace "replace_me" in a given file, instruct the user to add the "replace_me" placeholder in that file. Here's the user input:` + "\n\n"
	prompt += string(userRequest)

	res := send(string(prompt))
	printResponse(res)

	// iter := cs.SendMessageStream(ctx, genai.Text("Which one of those do you recommend?"))
	// for {
	// 	res, err := iter.Next()
	// 	if err == iterator.Done {
	// 		break
	// 	}
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	printResponse(res)
	// }

	// for i, c := range cs.History {
	// 	log.Printf("    %d: %+v", i, c)
	// }
	// res = send("Why do you like the Philips?")
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// printResponse(res)
	return nil
}

func printResponse(resp *genai.GenerateContentResponse) {
	for _, cand := range resp.Candidates {
		for _, part := range cand.Content.Parts {
			fmt.Println(part)
		}
	}
	fmt.Println("---")
}

func ptr[T any](t T) *T {
	return &t
}
