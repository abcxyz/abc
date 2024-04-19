package patch

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Patch struct {
	SourceName string
	DestName   string
	Hunks      []Hunk
}

type Hunk struct {
	SourceLineStart int
	SourceLength    int

	DestLineStart int
	DestLength    int

	BeforeContext string
	Actions       []Action
	AfterContext  string
}

// Either addition
type Action struct {
	// Will be "Add" if the diff line started with "+", or "Remove" if the line
	// started with "-".
	AddOrRemove AddOrRemove
	Line        string
}

type AddOrRemove int

const (
	Add AddOrRemove = iota + 1
	Remove
)

// // ParseUnifiedDiff parses a diff in the "unified diff" format into a data
// // structure.
// func ParseUnifiedDiff(diffContents []string) *Patch {

// }

// Parses the first two lines of a unified diff. Validates that they have the
// correct prefixes. Returns the source and destination filenames. The input
// must have length 2.
func parseMetadata(lines []string) (source, dest string, _ error) {
	panic("todo")
}

// ParseUnifiedDiff parses a diff in the "unified diff" format into a data
// structure.
func ParseUnifiedDiff(diffContents []string) (*Patch, error) {
	// The first three lines of a unified diff contain metadata about the diff.
	// The first line contains the source file name, the second line contains the
	// destination file name, and the third line contains the header information.
	if len(diffContents) < 3 {
		return nil, fmt.Errorf("invalid diff format: missing metadata")
	}

	// source, dest, err := parseMetadata(diffContents[0:2])
	// if err != nil {
	// 	return nil, err
	// }

	// The remaining lines of the diff contain the hunks. A hunk is a section of
	// the file that has been changed. Each hunk starts with a header line that
	// contains the source line start and length, and the destination line start
	// and length. The header line is followed by the actual changes, which are
	// represented as a series of "actions". An action is either an addition or a
	// removal.
	hunks := make([]Hunk, 0)
	for _, line := range diffContents[2:] {
		// If the line starts with "@@", then it is a hunk header.
		if strings.HasPrefix(line, "@@") {
			hunk, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			hunks = append(hunks, hunk)
		} else {
			// If the line does not start with "@@", then it is an action.
			action, err := parseAction(line)
			if err != nil {
				return nil, err
			}
			hunkIndex := len(hunks) - 1
			if hunkIndex < 0 {
				return nil, fmt.Errorf("got an action line before hunk beginning: %s", line)
			}

			hunks[hunkIndex].Actions = append(hunks[hunkIndex].Actions, action)
		}
	}

	return &Patch{
		SourceName: sourceName,
		DestName:   destName,
		Hunks:      hunks,
	}, nil
}

// parseHunkHeader parses a hunk header line.
func parseHunkHeader(line string) (Hunk, error) {
	// The hunk header line has the following format:
	//
	// @@ -source_line_start,source_length +dest_line_start,dest_length @@
	//
	// For example:
	//
	// @@ -1,3 +1,3 @@
	re := regexp.MustCompile(`@@ -(\d+),(\d+) \+(\d+),(\d+) @@`)
	matches := re.FindStringSubmatch(line)
	if matches == nil {
		return Hunk{}, fmt.Errorf("invalid hunk header: %s", line)
	}
	sourceLineStart, err := strconv.Atoi(matches[1])
	if err != nil {
		return Hunk{}, err
	}
	sourceLength, err := strconv.Atoi(matches[2])
	if err != nil {
		return Hunk{}, err
	}
	destLineStart, err := strconv.Atoi(matches[3])
	if err != nil {
		return Hunk{}, err
	}
	destLength, err := strconv.Atoi(matches[4])
	if err != nil {
		return Hunk{}, err
	}
	return Hunk{
		SourceLineStart: sourceLineStart,
		SourceLength:    sourceLength,
		DestLineStart:   destLineStart,
		DestLength:      destLength,
	}, nil
}

// parseAction parses an action line.
func parseAction(line string) (Action, error) {
	// An action line has the following format:
	//
	// +<line>
	// -<line>
	//
	// For example:
	//
	// +new line
	// -old line
	re := regexp.MustCompile(`^([+-])(.*)$`)
	matches := re.FindStringSubmatch(line)
	if matches == nil {
		return Action{}, fmt.Errorf("invalid action: %s", line)
	}
	addOrRemove := Add
	if matches[1] == "-" {
		addOrRemove = Remove
	}
	return Action{
		AddOrRemove: addOrRemove,
		Line:        matches[2],
	}, nil
}
