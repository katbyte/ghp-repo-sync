package gh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/katbyte/ghp-sync/lib/clog"
	"github.com/shurcooL/githubv4"
)

func (t Token) GraphQLQueryUnmarshal(query string, params [][]string, data any) error {
	out, err := t.GraphQLQuery(query, params)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(*out), data)
}

func (t Token) GraphQLQuery(query string, params [][]string) (*string, error) {
	const (
		maxAttempts = 5
		baseDelay   = time.Second
	)

	args := make([]string, 0, 4+2*len(params))
	args = append(args, "api", "graphql", "-f", query)

	for _, p := range params {
		args = append(args, p[0], p[1])
	}

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		ghc := exec.CommandContext(context.Background(), "gh", args...) //nolint:gosec // args are constructed internally

		// Preserve existing environment and add GITHUB_TOKEN if present
		env := os.Environ()
		if t.Token != nil {
			env = append(env, "GITHUB_TOKEN="+*t.Token)
		}
		ghc.Env = env

		out, err := ghc.CombinedOutput()
		outstr := string(out)

		// on success return the output immediately
		if err == nil {
			return &outstr, nil
		}

		// If it doesn't look like a rate limit or transient network error, fail fast
		retryable := isRateLimitError(outstr) || isRateLimitError(err.Error()) ||
			isTransientNetworkError(fmt.Errorf("%s: %w", outstr, err))
		if !retryable {
			return nil, fmt.Errorf("gh graphql failed: %w\noutput: %s", err, outstr)
		}

		// If we've used all attempts, bail out
		if attempt == maxAttempts {
			return nil, fmt.Errorf("still failing after %d attempts: %w\nlast output: %s", attempt, err, outstr)
		}

		// Exponential backoff (1s, 2s, 4s, 8s, ...)
		delay := baseDelay * time.Duration(math.Pow(2, float64(attempt-1)))
		time.Sleep(delay)
	}

	// Should be unreachable, but keeps compiler happy
	return nil, errors.New("gh graphql failed after retries, this should be unreachable")
}

// QueryWithRetry runs a githubv4 query, retrying transient network errors. The retryablehttp
// transport only guards the round-trip itself; errors while reading the response body (e.g.
// http2 stream resets mid-body: "stream error: stream ID N; CANCEL") surface here instead, so
// they need their own retry loop.
func QueryWithRetry(ctx context.Context, client *githubv4.Client, q any, variables map[string]any) error {
	const maxAttempts = 5

	var err error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err = client.Query(ctx, q, variables); err == nil {
			return nil
		}

		if !isTransientNetworkError(err) || attempt == maxAttempts {
			return err
		}

		delay := 2 * time.Second * time.Duration(math.Pow(2, float64(attempt-1)))
		clog.Log.Warnf("transient error querying github (attempt %d/%d), retrying in %s: %s", attempt, maxAttempts, delay, err)
		time.Sleep(delay)
	}

	return err
}

func isTransientNetworkError(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"stream error", // http2 stream reset (CANCEL, INTERNAL_ERROR, etc)
		"connection reset",
		"connection refused",
		"broken pipe",
		"unexpected eof",
		"timeout",
		"temporarily unavailable",
		"tls handshake",
		"no such host",
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func isRateLimitError(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "rate limit") ||
		strings.Contains(m, "api rate limit exceeded") ||
		strings.Contains(m, "secondary rate limit") ||
		strings.Contains(m, "abuse detection")
}
