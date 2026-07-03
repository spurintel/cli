package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/spurintel/cli/internal/spur"
)

// contextFlags carries the command-local flags for `spur context`. They are
// bound per invocation in newContextCmd rather than on the root command because
// they only make sense for this subcommand.
type contextFlags struct {
	extract     bool
	parallelism int
	date        string
}

// newContextCmd builds `spur context [ip ...]`: look up one or many IPs in the
// Spur Context API and render the results through the shared output renderer
// (styled text at a terminal, newline-delimited JSON when piped; overridable
// with --format).
//
// IPs come from positional arguments or, when none are given, from stdin
// (newline/whitespace/comma separated). --extract regex-pulls IPs out of
// freeform text on either source. Lookups run concurrently, bounded by
// --parallelism, and results render one per IP in input order.
func newContextCmd(of *outputFlags, cf *configFlags) *cobra.Command {
	var f contextFlags

	cmd := &cobra.Command{
		Use:   "context [ip ...]",
		Short: "Look up the Spur context for one or more IP addresses",
		Long: "Look up IPv4 or IPv6 addresses in the Spur Context API.\n\n" +
			"Pass IPs as arguments (spur context a b c) or pipe them on stdin " +
			"(newline, whitespace, or comma separated). Use --extract to pull IPs " +
			"out of freeform/garbage text. Lookups run concurrently and render one " +
			"report per IP, in input order.\n\n" +
			"Output is a styled report at a terminal and newline-delimited JSON when " +
			"piped or redirected; use --format to override.\n\n" +
			"Authentication uses the SPUR_TOKEN environment variable.",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dt, err := spur.ValidateHistoricalDate(f.date)
			if err != nil {
				return err
			}
			if f.parallelism < 1 {
				return fmt.Errorf("--parallelism must be at least 1")
			}

			ips, err := gatherIPs(cmd, args, f.extract)
			if err != nil {
				return err
			}
			if len(ips) == 0 {
				return errors.New("no IPs provided; pass IPs as arguments or pipe them on stdin")
			}

			token, err := resolveToken()
			if err != nil {
				return err
			}
			client := cf.newClient(token)
			opts := spur.IPContextOptions{DT: dt}

			results, failures := lookupContexts(cmd.Context(), client, ips, opts, f.parallelism)
			for _, fail := range failures {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", fail.ip, fail.err)
			}
			if len(results) == 0 {
				return fmt.Errorf("no IPs could be looked up")
			}
			return of.emit(cmd, results)
		},
	}

	cmd.Flags().BoolVar(&f.extract, "extract", false,
		"extract IP addresses from freeform/garbage text instead of treating input as clean IPs")
	cmd.Flags().IntVar(&f.parallelism, "parallelism", runtime.NumCPU()*2,
		"maximum number of concurrent lookups")
	cmd.Flags().StringVar(&f.date, "date", "",
		"historical lookup date in YYYYmmdd form (Enterprise subscriptions only)")

	return cmd
}

// resolveToken returns the Spur API token. For this cut it comes solely from
// the SPUR_TOKEN environment variable; config-file and keychain sources are
// deferred to a later ticket.
func resolveToken() (string, error) {
	if t := os.Getenv("SPUR_TOKEN"); t != "" {
		return t, nil
	}
	return "", errors.New("SPUR_TOKEN is not set; get a token from https://app.spur.us/settings/api")
}

// gatherIPs collects candidate IP strings from positional arguments or, when
// none are given, from piped stdin. Arguments take precedence over stdin so an
// explicit invocation is never surprised by leftover pipe input. --extract
// switches tokenization from clean-split to regex extraction and applies to
// whichever source is used.
func gatherIPs(cmd *cobra.Command, args []string, extract bool) ([]string, error) {
	var text string
	switch {
	case len(args) > 0:
		text = strings.Join(args, "\n")
	case stdinIsPiped():
		b, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		text = string(b)
	default:
		return nil, nil
	}

	if extract {
		return spur.ExtractIPs(text), nil
	}
	return spur.SplitCandidates(text), nil
}

// stdinIsPiped reports whether stdin is a pipe or redirect rather than an
// interactive terminal, so the command reads piped input but never blocks
// waiting on a terminal that has nothing to give.
func stdinIsPiped() bool {
	return !term.IsTerminal(int(os.Stdin.Fd()))
}

// lookupFailure is a candidate that could not be looked up (invalid IP or an
// API error), kept with its input so the command can report it per-IP.
type lookupFailure struct {
	ip  string
	err error
}

// lookupContexts looks up every IP concurrently, bounded by parallelism, and
// returns the successful contexts in input order plus the per-IP failures. The
// shared client is safe for concurrent use; each worker writes into a slot
// indexed by input position so the output order is deterministic regardless of
// which lookup finishes first.
func lookupContexts(ctx context.Context, client *spur.Client, ips []string, opts spur.IPContextOptions, parallelism int) ([]spur.IPContext, []lookupFailure) {
	type slot struct {
		ctx spur.IPContext
		err error
	}
	slots := make([]slot, len(ips))

	workers := parallelism
	if workers > len(ips) {
		workers = len(ips)
	}
	if workers < 1 {
		workers = 1
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				c, err := fetchContext(ctx, client, ips[i], opts)
				slots[i] = slot{ctx: c, err: err}
			}
		}()
	}
	for i := range ips {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	results := make([]spur.IPContext, 0, len(ips))
	var failures []lookupFailure
	for i, s := range slots {
		if s.err != nil {
			failures = append(failures, lookupFailure{ip: ips[i], err: s.err})
			continue
		}
		results = append(results, s.ctx)
	}
	return results, failures
}

// fetchContext validates the IP and calls the client, returning the context.
// It is split from the cobra wiring and from rendering so it can be exercised
// end-to-end against an httptest server and reused by every concurrent worker.
//
// API failures are collapsed to the client's stable, sanitized message
// (unauthorized / forbidden / throttled / bad request) so the operator sees a
// clear cause instead of a raw status line or a stack trace.
func fetchContext(ctx context.Context, client *spur.Client, ip string, opts spur.IPContextOptions) (spur.IPContext, error) {
	ip, err := spur.ValidateIP(ip)
	if err != nil {
		return spur.IPContext{}, err
	}

	result, _, err := client.GetIPContext(ctx, ip, opts)
	if err != nil {
		var apiErr *spur.APIError
		if errors.As(err, &apiErr) {
			return spur.IPContext{}, errors.New(apiErr.Message())
		}
		return spur.IPContext{}, err
	}
	return result, nil
}
