// Command pullout collects every known URL, path and endpoint for a single
// host by running the URL-discovery tools installed on the machine, then
// scoping, normalising and de-duplicating their combined output.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

type options struct {
	subs    bool
	paths   bool
	passive bool
	verbose bool
	timeout time.Duration
}

// result is one raw output line from a tool.
type result struct {
	tool string
	line string
}

func main() {
	var (
		opts     options
		only     string
		exclude  string
		listOnly bool
	)
	flag.BoolVar(&opts.subs, "subs", false, "also keep URLs on subdomains of the target")
	flag.BoolVar(&opts.paths, "paths", false, "print unique paths instead of full URLs")
	flag.BoolVar(&opts.passive, "passive", false, "archive sources only; never send traffic to the target")
	flag.BoolVar(&opts.verbose, "v", false, "show tool stderr and per-tool statistics")
	flag.DurationVar(&opts.timeout, "timeout", 10*time.Minute, "maximum run time per tool, per target")
	flag.StringVar(&only, "tools", "", "comma-separated tools to run (default: all installed)")
	flag.StringVar(&exclude, "exclude", "", "comma-separated tools to skip")
	flag.BoolVar(&listOnly, "list", false, "list supported tools and whether they are installed")
	flag.Usage = usage
	flag.Parse()

	if listOnly {
		listTools()
		return
	}

	selected, err := selectTools(only, exclude, opts.passive)
	if err != nil {
		fatal(err)
	}
	runnable := installed(selected)
	if len(runnable) == 0 {
		fatal(fmt.Errorf("none of the selected tools are installed; run `pullout -list` for install commands"))
	}

	targets, err := readTargets(flag.Args())
	if err != nil {
		fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	out := bufio.NewWriterSize(os.Stdout, 64<<10)
	defer out.Flush()

	failed := false
	for _, target := range targets {
		if ctx.Err() != nil {
			break
		}
		if !run(ctx, target, runnable, opts, out) {
			failed = true
		}
	}
	if failed || ctx.Err() != nil {
		out.Flush()
		os.Exit(1)
	}
}

// run executes every tool against one target concurrently and streams the
// scoped, de-duplicated results to out. It reports whether at least one tool
// completed successfully.
func run(ctx context.Context, target string, runnable []tool, opts options, out *bufio.Writer) bool {
	results := make(chan result, 1024)
	errs := make([]error, len(runnable))

	var wg sync.WaitGroup
	for i, t := range runnable {
		wg.Add(1)
		go func(i int, t tool) {
			defer wg.Done()
			errs[i] = t.run(ctx, target, opts, results)
		}(i, t)
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	// Single consumer: owns the dedupe set and the writer, so no locking.
	seen := make(map[string]struct{})
	counts := make(map[string]int)
	for r := range results {
		u, ok := normalize(r.line, target, opts.subs)
		if !ok {
			continue
		}
		counts[r.tool]++
		entry := u.String()
		if opts.paths {
			entry = u.EscapedPath()
		}
		if _, dup := seen[entry]; dup {
			continue
		}
		seen[entry] = struct{}{}
		out.WriteString(entry)
		out.WriteByte('\n')
		if len(results) == 0 {
			out.Flush()
		}
	}
	out.Flush()

	succeeded := false
	for i, t := range runnable {
		if errs[i] != nil {
			logf("%s: %s: %v", target, t.name, errs[i])
		} else {
			succeeded = true
		}
		if opts.verbose {
			logf("%s: %s: %d in-scope URLs", target, t.name, counts[t.name])
		}
	}
	if opts.verbose {
		logf("%s: %d unique results", target, len(seen))
	}
	return succeeded
}

// readTargets validates the targets given as arguments, or read one per line
// from stdin when there are none.
func readTargets(args []string) ([]string, error) {
	var raw []string
	if len(args) > 0 {
		raw = args
	} else {
		if stat, err := os.Stdin.Stat(); err == nil && stat.Mode()&os.ModeCharDevice != 0 {
			usage()
			os.Exit(2)
		}
		sc := bufio.NewScanner(io.LimitReader(os.Stdin, 16<<20))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				raw = append(raw, line)
			}
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
	}

	var targets []string
	seen := make(map[string]struct{})
	for _, r := range raw {
		host, err := parseTarget(r)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[host]; !dup {
			seen[host] = struct{}{}
			targets = append(targets, host)
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("no targets given")
	}
	return targets, nil
}

func listTools() {
	for _, t := range tools {
		status := "missing   " + t.install
		if path, ok := t.lookPath(); ok {
			status = "installed " + path
		}
		kind := "passive"
		if t.active {
			kind = "active"
		}
		fmt.Printf("%-12s %-8s %s\n", t.name, kind, status)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `pullout - pull every known URL, path and endpoint out of a host

Usage:
  pullout [flags] <domain> [domain...]
  cat hosts.txt | pullout [flags]

Flags:
`)
	flag.PrintDefaults()
}

func logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "pullout: "+format+"\n", args...)
}

func fatal(err error) {
	logf("%v", err)
	os.Exit(1)
}
