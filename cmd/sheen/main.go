// Package main provides the entry point for the Sheen CLI application.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/hangarbay/sheen/internal/render"
	"github.com/hangarbay/sheen/internal/ui"

	"github.com/muesli/termenv"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// Version is set at build time.
var Version = "0.1.0"

var (
	styleFlag string
	widthFlag int
	pagerFlag bool
	tuiFlag   bool
	linksFlag string

	rootCmd = &cobra.Command{
		Use:   "sheen [SOURCE|DIR]",
		Short: "Render HTML in the terminal, with pizzazz!",
		Long: "Render HTML in the terminal, with pizzazz!\n\n" +
			"SOURCE can be a local file, a URL, or - for stdin. When given a\n" +
			"directory, sheen looks for an index.html inside it.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: false,
		ValidArgsFunction: func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return nil, cobra.ShellCompDirectiveDefault
		},
		RunE: execute,
	}
)

// source provides a readable HTML source.
type source struct {
	reader   io.ReadCloser
	URL      *url.URL
	isRemote bool
}

var domainRe = regexp.MustCompile(
	`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+(:\d+)?(/|$)`)

func sourceFromArg(arg string) (*source, error) {
	if arg == "-" {
		return &source{reader: os.Stdin}, nil
	}
	if u, err := url.Parse(arg); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
		return remoteSource(u)
	}
	// local paths win over domain-looking names, so "notes.html" opens the
	// file even though it looks like a hostname
	if st, err := os.Stat(arg); err == nil {
		if st.IsDir() {
			idx := findIndexHTML(arg)
			if idx == "" {
				return nil, fmt.Errorf("no index.html found in %s", arg)
			}
			return fileSource(idx)
		}
		return fileSource(arg)
	}
	if domainRe.MatchString(arg) {
		if u, err := url.Parse("https://" + arg); err == nil {
			return remoteSource(u)
		}
	}
	return fileSource(arg)
}

func remoteSource(u *url.URL) (*source, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(u.String())
	if err != nil {
		return nil, fmt.Errorf("unable to fetch url: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP status %d fetching %s", resp.StatusCode, u)
	}
	return &source{reader: resp.Body, URL: u, isRemote: true}, nil
}

func fileSource(path string) (*source, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open file: %w", err)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("unable to resolve path: %w", err)
	}
	return &source{
		reader: f,
		URL:    &url.URL{Scheme: "file", Path: filepath.Dir(abs) + "/"},
	}, nil
}

// findIndexHTML picks index.html (or index.htm, case-insensitive) from a
// directory, falling back to the first HTML file found.
func findIndexHTML(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var fallback string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := strings.ToLower(e.Name())
		if name != "index.html" && name != "index.htm" {
			if fallback == "" && (strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".htm")) {
				fallback = filepath.Join(dir, e.Name())
			}
			continue
		}
		return filepath.Join(dir, e.Name())
	}
	return fallback
}

func isTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

func stdinIsPipe() (bool, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false, err
	}
	return stat.Mode()&os.ModeCharDevice == 0 || stat.Size() > 0, nil
}

func resolveStyle() string {
	switch styleFlag {
	case "dark", "light", "notty", "ascii":
		return styleFlag
	}
	if !isTTY(os.Stdout) {
		return "notty"
	}
	if termenv.HasDarkBackground() {
		return "dark"
	}
	return "light"
}

func resolveLinks() string {
	switch linksFlag {
	case "osc8", "inline", "none":
		return linksFlag
	}
	if isTTY(os.Stdout) {
		return "osc8"
	}
	return "inline"
}

func resolveWidth() int {
	if widthFlag > 0 {
		return widthFlag
	}
	if isTTY(os.Stdout) {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
			return w
		}
	}
	return 80
}

func stylesheetFetcher(src *source) func(string) ([]byte, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	return func(raw string) ([]byte, error) {
		u, err := url.Parse(raw)
		if err != nil {
			return nil, err
		}
		switch u.Scheme {
		case "http", "https":
			resp, err := client.Get(u.String())
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
			}
			return io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		case "file":
			if src.isRemote {
				return nil, errors.New("remote page may not load local stylesheets")
			}
			if u.Path == "" {
				return nil, errors.New("empty stylesheet path")
			}
			return os.ReadFile(u.Path)
		}
		return nil, fmt.Errorf("unsupported stylesheet scheme %q", u.Scheme)
	}
}

func execute(cmd *cobra.Command, args []string) error {
	if yes, err := stdinIsPipe(); err != nil {
		return err
	} else if yes {
		src := &source{reader: os.Stdin}
		defer src.reader.Close() //nolint:errcheck
		return executeCLI(src)
	}
	if len(args) == 0 {
		return cmd.Help()
	}
	src, err := sourceFromArg(args[0])
	if err != nil {
		return err
	}
	defer src.reader.Close() //nolint:errcheck
	return executeCLI(src)
}

func executeCLI(src *source) error {
	b, err := io.ReadAll(src.reader)
	if err != nil {
		return fmt.Errorf("unable to read source: %w", err)
	}

	opts := render.Options{
		Width:           resolveWidth(),
		Preset:          resolveStyle(),
		BaseURL:         src.URL,
		Links:           resolveLinks(),
		FetchStylesheet: stylesheetFetcher(src),
	}

	switch {
	case pagerFlag && tuiFlag:
		return errors.New("cannot use both pager and tui")
	case tuiFlag:
		_, err := ui.NewProgram(ui.Config{
			HTML:            string(b),
			Source:          sourceLabel(src),
			Preset:          opts.Preset,
			Links:           opts.Links,
			BaseURL:         opts.BaseURL,
			FetchStylesheet: opts.FetchStylesheet,
		}).Run()
		return err
	default:
		out, err := render.Render(bytes.NewReader(b), opts)
		if err != nil {
			return fmt.Errorf("unable to render HTML: %w", err)
		}
		if pagerFlag {
			return runPager(out)
		}
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		_, err = fmt.Fprint(os.Stdout, out)
		return err
	}
}

func sourceLabel(src *source) string {
	if src.isRemote {
		return src.URL.String()
	}
	if src.URL != nil {
		return filepath.Base(strings.TrimSuffix(src.URL.Path, "/"))
	}
	return "(stdin)"
}

func runPager(out string) error {
	pagerCmd := os.Getenv("PAGER")
	if pagerCmd == "" {
		pagerCmd = "less -r"
	}
	fields := strings.Fields(pagerCmd)
	if len(fields) == 0 {
		return fmt.Errorf("unable to parse PAGER command: %s", pagerCmd)
	}
	c := exec.Command(fields[0], fields[1:]...) //nolint:gosec
	c.Stdin = strings.NewReader(out)
	c.Stdout = os.Stdout
	if err := c.Run(); err != nil {
		return fmt.Errorf("unable to run pager: %w", err)
	}
	return nil
}

func main() {
	rootCmd.Version = Version
	rootCmd.Flags().StringVarP(&styleFlag, "style", "s", "auto", "style name (auto, dark, light, notty, ascii)")
	rootCmd.Flags().IntVarP(&widthFlag, "width", "w", 0, "word-wrap width (default: terminal width)")
	rootCmd.Flags().BoolVarP(&pagerFlag, "pager", "p", false, "display in an external pager ($PAGER, default less -r)")
	rootCmd.Flags().BoolVarP(&tuiFlag, "tui", "t", false, "display in the interactive scrollable TUI")
	rootCmd.Flags().StringVar(&linksFlag, "links", "auto", "link rendering (auto, osc8, inline, none)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
