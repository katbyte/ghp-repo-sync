package gh

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/google/go-github/v89/github"
	"github.com/katbyte/ghp-sync/lib/clog"
)

func (r Repo) PrURL(pr int) string {
	return "https://github.com/" + r.Owner + "/" + r.Name + "/pull/" + strconv.Itoa(pr)
}

func (r Repo) ListAllPullRequests(state string, cb func([]*github.PullRequest, *github.Response) error) error {
	client, ctx := r.NewClient()

	opts := &github.PullRequestListOptions{
		State: state,
		ListOptions: github.ListOptions{
			Page:    1,
			PerPage: 100,
		},
	}

	for {
		clog.Log.Debugf("Listing all PRs for %s/%s (Page %d)...", r.Owner, r.Name, opts.Page)
		prs, resp, err := client.PullRequests.List(ctx, r.Owner, r.Name, opts)
		if err != nil {
			return fmt.Errorf("unable to list PRs for %s/%s (Page %d): %w", r.Owner, r.Name, opts.Page, err)
		}

		if err = cb(prs, resp); err != nil {
			return fmt.Errorf("callback failed for %s/%s (Page %d): %w", r.Owner, r.Name, opts.Page, err)
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return nil
}

func (r Repo) GetAllPullRequests(state string) (*[]github.PullRequest, error) {
	var allPRs []github.PullRequest

	err := r.ListAllPullRequests(state, func(prs []*github.PullRequest, resp *github.Response) error {
		for i, p := range prs {
			if p == nil {
				clog.Log.Debugf("prs[%d] was nil, skipping", i)
				continue
			}

			n := p.GetNumber()
			if n == 0 {
				clog.Log.Debugf("prs[%d].Number was nil/0, skipping", i)
				continue
			}

			allPRs = append(allPRs, *p)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get all prs for %s/%s: %w", r.Owner, r.Name, err)
	}

	sort.Slice(allPRs, func(i, j int) bool {
		return allPRs[i].GetNumber() < allPRs[j].GetNumber()
	})

	return &allPRs, nil
}

// GetPullRequestReviews returns all reviews for a PR in submission order (oldest first).
func (r Repo) GetPullRequestReviews(pr int) ([]*github.PullRequestReview, error) {
	client, ctx := r.NewClient()

	var all []*github.PullRequestReview
	opts := &github.ListOptions{PerPage: 100}
	for {
		reviews, resp, err := client.PullRequests.ListReviews(ctx, r.Owner, r.Name, pr, opts)
		if err != nil {
			return nil, fmt.Errorf("unable to list reviews for PR %d in %s/%s: %w", pr, r.Owner, r.Name, err)
		}
		all = append(all, reviews...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return all, nil
}

// GetPullRequestReviewComments returns all review (inline) comments for a PR; each carries the ID
// of the review it was submitted with in PullRequestReviewID.
func (r Repo) GetPullRequestReviewComments(pr int) ([]*github.PullRequestComment, error) {
	client, ctx := r.NewClient()

	var all []*github.PullRequestComment
	opts := &github.PullRequestListCommentsOptions{ListOptions: github.ListOptions{PerPage: 100}}
	for {
		comments, resp, err := client.PullRequests.ListComments(ctx, r.Owner, r.Name, pr, opts)
		if err != nil {
			return nil, fmt.Errorf("unable to list review comments for PR %d in %s/%s: %w", pr, r.Owner, r.Name, err)
		}
		all = append(all, comments...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return all, nil
}

func (r Repo) GetPullRequest(pr int) (*github.PullRequest, error) {
	client, ctx := r.NewClient()

	p, _, err := client.PullRequests.Get(ctx, r.Owner, r.Name, pr)
	if err != nil {
		return nil, fmt.Errorf("unable to get PR %d for %s/%s: %w", pr, r.Owner, r.Name, err)
	}

	return p, nil
}
