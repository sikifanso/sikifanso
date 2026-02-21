package prompt

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// String prompts the user with a label and a default value. If the user
// presses Enter without typing anything, the default is returned.
// Output goes to stderr (project convention: UI output goes to stderr).
func String(label, defaultVal string) string {
	fmt.Fprintf(os.Stderr, "  %s (%s): ", label, defaultVal)

	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text != "" {
			return text
		}
	}
	return defaultVal
}
