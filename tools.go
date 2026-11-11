package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// maxLine bounds a single output line. Anything longer is not a usable URL and
// is skipped rather than buffered.
const maxLine = 64 << 10

// tool describes one external URL-discovery binary.
type tool struct {
	name    string
	install string
	// active marks tools that send traffic to the target itself.
	active bool
	// args builds the argument vector for a validated host.
	args func(host string, subs bool) []string
}

// tools is the registry. Every entry prints one URL per line on stdout.
var tools = []tool{
	{
		name:    "gau",
		install: "go install github.com/lc/gau/v2/cmd/gau@latest",
		args: func(host string, subs bool) []string {
			a := []string{"--threads", "5"}
			if subs {
				a = append(a, "--subs")
			}
			return append(a, host)
		},
	},
	{
		name:    "waybackurls",
		install: "go install github.com/tomnomnom/waybackurls@latest",
		args: func(host string, subs bool) []string {
			if subs {
				return []string{host}
			}
			return []string{"-no-subs", host}
		},
	},
	{
		name:    "gowaybackgo",
		install: "go install github.com/OoS-MaMaD/gowaybackgo@latest",
		args: func(host string, subs bool) []string {
			if subs {
				host = "*." + host
			}
			// Several tools share the Wayback CDX API from one IP; stay
			// well under its rate limit to avoid 429s.
			return []string{"-u", host, "--rate", "3", "--page-workers", "3"}
		},
	},
	{
		name:    "waymore",
		install: "pipx install waymore",
		args: func(host string, subs bool) []string {
			a := []string{"-i", host, "-mode", "U", "--stream"}
			if !subs {
				a = append(a, "-n")
			}
			return a
		},
	},
	{
		name:    "urlfinder",
		install: "go install github.com/projectdiscovery/urlfinder/cmd/urlfinder@latest",
		args: func(host string, subs bool) []string {
			return []string{"-d", host, "-silent", "-nc", "-duc", "-fs", fieldScope(subs)}
		},
	},
	{
		name:    "katana",
		install: "go install github.com/projectdiscovery/katana/cmd/katana@latest",
		active:  true,
		args: func(host string, subs bool) []string {
			return []string{
				"-u", "https://" + host,
				"-silent", "-nc", "-duc",
				"-d", "3", "-jc", "-kf", "all",
				"-fs", fieldScope(subs),
			}
		},
	},
}

// fieldScope maps the scope to the ProjectDiscovery -fs value. "rdn" is wider
// than "subdomains of host" when host is itself a subdomain; normalize()
// enforces the exact scope on the output.
func fieldScope(subs bool) string {
	if subs {
		return "rdn"
	}
	return "fqdn"
}

func (t tool) lookPath() (string, bool) {
	path, err := exec.LookPath(t.name)
	return path, err == nil
}

// selectTools applies the -tools, -exclude and -passive flags to the registry.
func selectTools(only, exclude string, passive bool) ([]tool, error) {
	known := make(map[string]struct{}, len(tools))
	for _, t := range tools {
		known[t.name] = struct{}{}
	}
	parse := func(csv string) (map[string]struct{}, error) {
		set := make(map[string]struct{})
		for _, n := range strings.Split(csv, ",") {
			n = strings.ToLower(strings.TrimSpace(n))
			if n == "" {
				continue
			}
			if _, ok := known[n]; !ok {
				return nil, fmt.Errorf("unknown tool %q; run `pullout -list`", n)
			}
			set[n] = struct{}{}
		}
		return set, nil
	}

	want, err := parse(only)
	if err != nil {
		return nil, err
	}
	skip, err := parse(exclude)
	if err != nil {
		return nil, err
	}

	var selected []tool
	for _, t := range tools {
		if _, ok := want[t.name]; len(want) > 0 && !ok {
			continue
		}
		if _, ok := skip[t.name]; ok {
			continue
		}
		if passive && t.active {
			continue
		}
		selected = append(selected, t)
	}
	if len(selected) == 0 {
		return nil, errors.New("no tools selected")
	}
	return selected, nil
}

// installed returns the selected tools found on PATH, reporting the rest.
func installed(selected []tool) []tool {
	var runnable []tool
	for _, t := range selected {
		if _, ok := t.lookPath(); ok {
			runnable = append(runnable, t)
			continue
		}
		logf("%s not installed, skipping (%s)", t.name, t.install)
	}
	return runnable
}

// run executes the tool against host and streams each stdout line to out.
// host must already be validated by parseTarget: it is passed as a bare
// argument and must never be able to act as a flag.
func (t tool) run(ctx context.Context, host string, opts options, out chan<- result) error {
	ctx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, t.name, t.args(host, opts.subs)...)
	cmd.Stdin = nil
	cmd.WaitDelay = 5 * time.Second
	isolate(cmd)
	if opts.verbose {
		cmd.Stderr = os.Stderr
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	readErr := scanLines(stdout, func(line string) {
		out <- result{tool: t.name, line: line}
	})
	waitErr := cmd.Wait()

	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("stopped after %s timeout (partial results kept)", opts.timeout)
	case ctx.Err() != nil:
		return ctx.Err()
	case waitErr != nil:
		return waitErr
	}
	return readErr
}

// scanLines calls fn for every line of r, skipping lines longer than maxLine
// instead of failing, so one oversized record cannot stall the child process.
func scanLines(r io.Reader, fn func(string)) error {
	br := bufio.NewReaderSize(r, maxLine)
	for {
		line, isPrefix, err := br.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, os.ErrClosed) {
				return nil
			}
			return err
		}
		if isPrefix {
			for isPrefix && err == nil {
				_, isPrefix, err = br.ReadLine()
			}
			continue
		}
		fn(string(line))
	}
}
