package common

// Shows the TagGroups stored on the server

import (
	"strings"

	fzf "github.com/junegunn/fzf/src"
)

func FzfMapOfStringArray(input map[string][]string) []string {
	if input != nil {
		inputChan := make(chan string)
		go func() {
			for k, v := range input {
				inputChan <- k + "|" + strings.Join(v, ",")
			}
			close(inputChan)
		}()

		output := []string{}
		outputChan := make(chan string)
		go func() {
			for s := range outputChan {
				output = append(output, s)
			}
		}()

		options, _ := fzf.ParseOptions(
			true, // whether to load defaults ($FZF_DEFAULT_OPTS_FILE and $FZF_DEFAULT_OPTS)
			// I need to make fzf editor something that is configured!
			[]string{"--border", "--reverse", "--delimiter=[|]", "--with-nth", "..1", "--preview", "echo {2}"},
		)

		// Set up input and output channels
		options.Input = inputChan
		options.Output = outputChan

		// Run fzf
		fzf.Run(options)

		return output
	} else {
		return []string{}
	}
}

func FzfMapOfString(input map[string]string) []string {
	if input != nil {
		inputChan := make(chan string)
		go func() {
			for k, v := range input {
				inputChan <- k + "|" + v
			}
			close(inputChan)
		}()

		output := []string{}
		outputChan := make(chan string)
		go func() {
			for s := range outputChan {
				output = append(output, s)
			}
		}()

		options, _ := fzf.ParseOptions(
			true, // whether to load defaults ($FZF_DEFAULT_OPTS_FILE and $FZF_DEFAULT_OPTS)
			// I need to make fzf editor something that is configured!
			[]string{"--border", "--reverse", "--delimiter=[|]", "--with-nth", "..1", "--preview", "echo {2}"},
		)

		// Set up input and output channels
		options.Input = inputChan
		options.Output = outputChan

		// Run fzf
		fzf.Run(options)

		return output
	} else {
		return []string{}
	}
}
