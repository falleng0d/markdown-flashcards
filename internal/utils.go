package internal

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"
)

// ClearConsole Moves the cursor to the home position (0,0) and erases everything from cursor to end of screen.
func ClearConsole() {
	if os.Getenv("DEBUG") != "true" {
		fmt.Print("\033[H\033[0J")
	}
}

// ScrollDownScreen scrolls down by printing newlines. This can be helpful to prevent overwriting previous console output
// when clearing the console.
func ScrollDownScreen() {
	_, height, err := term.GetSize(int(os.Stdout.Fd()))
	check(err)
	for i := 0; i < height; i++ {
		fmt.Println()
	}
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

// PrintJSON pretty prints any struct as JSON
func PrintJSON[T any](v T) {
	out, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(out))
}

// ReadNumberInput reads a number from standard input. The number must be within i and j.
// It now accepts a single digit immediately when the user presses the key (no Enter required).
// If the terminal cannot be switched to raw mode it falls back to the previous behavior.
func ReadNumberInput(i, j int) int {
	res := i - 1

	// Try to switch the terminal into raw mode so we can read a single keypress.
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		// Fallback to line-based input if raw mode isn't available.
		scanner := bufio.NewScanner(os.Stdin)
		for res < i || res > j {
			scanner.Scan()
			in := scanner.Text()
			nr, err := strconv.Atoi(in)
			if err != nil || nr < i || nr > j {
				fmt.Print("Please enter a number: ")
				continue
			}
			res = nr
		}
		return res
	}
	// Ensure terminal state is restored.
	defer func() { _ = term.Restore(fd, oldState) }()

	buf := make([]byte, 1)
	for res < i || res > j {
		fmt.Print("Please enter a number: ")
		// Read a single byte (key press).
		n, err := os.Stdin.Read(buf)
		if err != nil || n == 0 {
			continue
		}
		b := buf[0]
		// Accept only ASCII digits 0-9.
		if b >= '0' && b <= '9' {
			nr := int(b - '0')
			// Echo the pressed key and newline so the user sees their input.
			fmt.Println(string(b))
			if nr >= i && nr <= j {
				res = nr
				break
			}
			// If out of range, loop and prompt again.
			continue
		}
		// Ignore other keys and keep waiting for a digit.
	}

	return res
}

// ReadEnterInput Blocks until the user enters a newline.
func ReadEnterInput() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Scan()
}

// CompareCategory compares the category name to the user input and returns true if the input matches with the
// category according to the following rules:
// - If the input is empty, it will match with any category.
// - The category and input get transformed to lowercase.
// - The input matches the category either if it is equal or if it is a prefix of the category.
func CompareCategory(category, input string) bool {
	if input == "" {
		return true
	}
	category = strings.ToLower(category)
	input = strings.ToLower(input)
	return strings.HasPrefix(category, input)
}

// FindClosestDate finds the closest due date in the future in the given slice of cards.
// If it contains a date that is before or equal to today, it will return an error.
func FindClosestDate(cards []Card) (time.Time, error) {
	y, m, d := time.Now().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	var closestDate time.Time
	for _, c := range cards {
		if c.Due.Before(today) || c.Due.Equal(today) {
			return time.Time{}, errors.New("found due date in the past")
		}
		if closestDate.IsZero() || c.Due.Before(closestDate) {
			closestDate = c.Due
		}
	}
	return closestDate, nil
}

// WrapLines wraps the given string into lines of the given length.
// It will not break words and thus only breaks at whitespace. It assumes that no word in the given string exceeds the
// requested line length. Lines that start with an indent will be indented by the given indent plus, if the line is
// a list item, the length of the list item prefix.
//
// If the lineLength is 0, it will wrap the text depending on the terminal width.
func WrapLines(s string, lineLength uint) string {
	if lineLength == 0 {
		width, _, err := term.GetSize(int(os.Stdout.Fd()))
		check(err)
		lineLength = uint(width)
	}

	lineFeedRegex := regexp.MustCompile("\r?\n")
	indentRegex := regexp.MustCompile(`(?m)^[\-+*\d.\s]+`)
	lineBreakRegex := regexp.MustCompile(fmt.Sprintf(`(?m)^.{1,%d}\s`, lineLength))

	var result string
	lines := lineFeedRegex.Split(s, -1)
	for _, line := range lines {
		if len(line) == 0 {
			result += "\n"
		}

		linePrefix := indentRegex.FindString(line)
		lineIndent := len(linePrefix)
		for len(line) > 0 {
			if uint(len(line)) <= lineLength {
				result += line + "\n"
				break
			} else {
				idx := len(line)
				idxs := lineBreakRegex.FindStringIndex(line)
				if idxs != nil {
					idx = idxs[1]
				}
				result += line[:idx] + "\n"
				remainder := strings.TrimSpace(line[idx:])
				remainderLen := len([]rune(remainder))
				paddingFmt := fmt.Sprintf("%%%ds", lineIndent+remainderLen)
				line = fmt.Sprintf(paddingFmt, remainder)
			}
		}
	}

	return result
}

// FormatMarkdown converts a small subset of Markdown (bold, italics, links) to console-friendly output.
// - Bold: **text** or __text__ -> ANSI bold
// - Italic: *text* or _text_ -> ANSI italic
// - Links: [label](url) -> "label (url)"
//
// This implementation is intentionally small and does not attempt to fully parse Markdown.
// It performs simple regex-based replacements which are sufficient for basic formatting.
func FormatMarkdown(s string) string {
	// Convert links first so we don't accidentally format parts of the URL as bold/italic.
	linkRe := regexp.MustCompile(`(?s)\[([^\]]+)\]\(([^)]+)\)`)
	s = linkRe.ReplaceAllString(s, "$1 ($2)")

	// Bold: **text** and __text__ (two separate, since Go regexp does not support backreferences)
	boldRe1 := regexp.MustCompile(`(?s)\*\*(.+?)\*\*`)
	s = boldRe1.ReplaceAllString(s, "\033[1m$1\033[0m")
	boldRe2 := regexp.MustCompile(`(?s)__(.+?)__`)
	s = boldRe2.ReplaceAllString(s, "\033[1m$1\033[0m")

	// Italic: *text* and _text_
	// Run after bold so **...** / __...__ are already handled.
	italicRe1 := regexp.MustCompile(`(?s)\*(.+?)\*`)
	s = italicRe1.ReplaceAllString(s, "\033[3m$1\033[0m")
	italicRe2 := regexp.MustCompile(`(?s)_(.+?)_`)
	s = italicRe2.ReplaceAllString(s, "\033[3m$1\033[0m")

	return s
}
