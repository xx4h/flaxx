package extractor

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Choice captures the user's decision for a single candidate variable.
type Choice struct {
	// Keep is true if the candidate should be turned into a variable.
	Keep bool
	// Name is the variable name to use (may differ from the candidate's
	// suggested name if the user renamed it).
	Name string
}

// Prompter drives interactive mode. Implementations may be terminal-backed
// (stdlib bufio.Reader) or scripted for testing.
type Prompter interface {
	// Ask walks through the candidate and returns the user's decision.
	Ask(c Candidate) (Choice, error)
}

// NewStdinPrompter returns a Prompter that reads from r and writes to w.
func NewStdinPrompter(r io.Reader, w io.Writer) Prompter {
	return &stdinPrompter{reader: bufio.NewReader(r), writer: w}
}

type stdinPrompter struct {
	reader *bufio.Reader
	writer io.Writer
}

func (p *stdinPrompter) Ask(c Candidate) (Choice, error) {
	for {
		fmt.Fprintf(p.writer,
			"Parameterize %q as {{.%s}} (%s)? [Y/n/r=rename] ",
			c.Value, c.Name, c.Description,
		)
		line, err := p.reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return Choice{}, err
		}
		answer := strings.TrimSpace(strings.ToLower(line))

		switch answer {
		case "", "y", "yes":
			return Choice{Keep: true, Name: c.Name}, nil
		case "n", "no":
			return Choice{Keep: false, Name: c.Name}, nil
		case "r", "rename":
			fmt.Fprintf(p.writer, "  new name for %q: ", c.Value)
			nameLine, readErr := p.reader.ReadString('\n')
			if readErr != nil && readErr != io.EOF {
				return Choice{}, readErr
			}
			newName := strings.TrimSpace(nameLine)
			if newName == "" {
				fmt.Fprintln(p.writer, "  name cannot be empty")
				continue
			}
			return Choice{Keep: true, Name: newName}, nil
		default:
			fmt.Fprintln(p.writer, "  please answer y, n, or r")
		}
	}
}
