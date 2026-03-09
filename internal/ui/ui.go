package ui

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorCyan   = "\033[36m"
	colorBold   = "\033[1m"
)

func Success(format string, args ...interface{}) {
	fmt.Printf(colorGreen+"✓ "+colorReset+format+"\n", args...)
}

func Info(format string, args ...interface{}) {
	fmt.Printf(colorBlue+"→ "+colorReset+format+"\n", args...)
}

func Warn(format string, args ...interface{}) {
	fmt.Printf(colorYellow+"! "+colorReset+format+"\n", args...)
}

func Error(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, colorRed+"✗ "+colorReset+format+"\n", args...)
}

func Step(n int, total int, format string, args ...interface{}) {
	prefix := fmt.Sprintf(colorCyan+"[%d/%d]"+colorReset+" ", n, total)
	fmt.Printf(prefix+format+"\n", args...)
}

func Bold(s string) string {
	return colorBold + s + colorReset
}

func StatusColor(status string) string {
	switch status {
	case "running":
		return colorGreen + status + colorReset
	case "stopped", "error":
		return colorRed + status + colorReset
	case "created", "pending", "deploying":
		return colorYellow + status + colorReset
	default:
		return status
	}
}

func Table(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, colorBold+strings.Join(headers, "\t")+colorReset)
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}
