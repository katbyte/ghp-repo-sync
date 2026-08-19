package gh

import (
	"fmt"
	"strings"
	"time"

	"github.com/shurcooL/githubv4"
)

type ClosingIssue struct {
	NodeID string
	Number int
}

type PullRequest struct {
	NodeID                     string
	Author                     string
	Number                     int
	Title                      string
	State                      string
	ReviewDecision             string
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
	ClosedAt                   time.Time
	MergedAt                   time.Time
	MergedBy                   string
	Draft                      bool
	Milestone                  string
	TotalCommentCount          int
	TotalReviewCount           int
	ReviewCommentCount         int
	FilteredReviewCount        int
	FilteredReviewCommentCount int

	ClosingIssues            []ClosingIssue
	Assignees                []string
	AssociatedLabels         map[string]bool
	AssociatedProjectNumbers map[int]bool
}

type pullRequestsQuery struct {
	Repository struct {
		PullRequests struct {
			Nodes []struct {
				ID                 string
				Number             int
				Title              string
				State              string
				ReviewDecision     string
				CreatedAt          time.Time
				UpdatedAt          time.Time
				ClosedAt           time.Time
				MergedAt           time.Time
				IsDraft            bool
				TotalCommentsCount int

				Assignees struct {
					Nodes []struct {
						Login string
					}
				} `graphql:"assignees(first: 10)"`

				Author struct {
					Login string
				}

				MergedBy struct {
					Login string
				}

				Labels struct {
					Nodes []struct {
						Name string
					}
				} `graphql:"labels(first: 100)"`

				Milestone struct {
					Title string
				}

				Reviews struct {
					Nodes []struct {
						Author struct {
							Login string
						}
						Comments struct {
							TotalCount int
						}
						State string
					}
				} `graphql:"reviews(first: 100)"`

				ProjectItems struct {
					Nodes []struct {
						Project struct {
							Number int
						}
					}
				} `graphql:"projectItems(first: 10)"`

				ClosingIssuesReferences struct {
					Nodes []struct {
						ID     string
						Number int
					}
				} `graphql:"closingIssuesReferences(first: 10)"`
			}

			PageInfo struct {
				EndCursor   string
				HasNextPage bool
			}
		} `graphql:"pullRequests(first: 40, after: $cursor, states: $state, orderBy: $orderBy)"`
	} `graphql:"repository(owner: $owner, name: $repository)"`
}

// GetAllPullRequestsGQL retrieves all pull requests matching the given states. If mergedSince is
// set only PRs merged at or after that time are returned, and pagination walks PRs by most
// recently updated so it can stop once it reaches PRs untouched since then (a PR's updatedAt is
// always >= its mergedAt).
func (r Repo) GetAllPullRequestsGQL(states, reviewers []string, limit int, mergedSince *time.Time, progress func(int)) (*[]PullRequest, error) {
	client, ctx, err := r.NewGraphQLClient()
	if err != nil {
		return nil, fmt.Errorf("instantiating GraphQL client: %w", err)
	}

	allPRs := make([]PullRequest, 0)

	rev := make(map[string]struct{})
	if len(reviewers) != 0 {
		for _, reviewer := range reviewers {
			rev[strings.ToLower(reviewer)] = struct{}{}
		}
	}

	ghStates := make([]githubv4.PullRequestState, 0, len(states))
	for _, state := range states {
		ghStates = append(ghStates, githubv4.PullRequestState(state))
	}

	orderBy := githubv4.IssueOrder{Field: githubv4.IssueOrderFieldCreatedAt, Direction: githubv4.OrderDirectionDesc}
	if mergedSince != nil {
		orderBy.Field = githubv4.IssueOrderFieldUpdatedAt
	}

	query := pullRequestsQuery{}
	variables := map[string]any{
		"owner":      githubv4.String(r.Owner),
		"repository": githubv4.String(r.Name),
		"state":      ghStates,
		"orderBy":    orderBy,
		"cursor":     (*githubv4.String)(nil), // Default to nil / null, conditionally update this if there is pagination
	}

	for {
		if err := QueryWithRetry(ctx, client, &query, variables); err != nil {
			return nil, err
		}

		prs := query.flatten(rev)
		if mergedSince != nil {
			kept := prs[:0]
			for _, pr := range prs {
				if !pr.MergedAt.Before(*mergedSince) {
					kept = append(kept, pr)
				}
			}
			prs = kept
		}
		allPRs = append(allPRs, prs...)

		if progress != nil {
			progress(len(allPRs))
		}

		if !query.Repository.PullRequests.PageInfo.HasNextPage || (limit > 0 && len(allPRs) >= limit) {
			break
		}

		// ordered by UPDATED_AT DESC, so once a page ends before mergedSince no later PR can match
		if mergedSince != nil {
			nodes := query.Repository.PullRequests.Nodes
			if len(nodes) > 0 && nodes[len(nodes)-1].UpdatedAt.Before(*mergedSince) {
				break
			}
		}

		variables["cursor"] = githubv4.String(query.Repository.PullRequests.PageInfo.EndCursor)
	}

	return &allPRs, nil
}

func (q pullRequestsQuery) flatten(reviewers map[string]struct{}) []PullRequest {
	result := make([]PullRequest, 0, len(q.Repository.PullRequests.Nodes))

	for _, pullRequest := range q.Repository.PullRequests.Nodes {
		pr := PullRequest{
			NodeID:                   pullRequest.ID,
			Author:                   pullRequest.Author.Login,
			Number:                   pullRequest.Number,
			Title:                    pullRequest.Title,
			State:                    pullRequest.State,
			ReviewDecision:           pullRequest.ReviewDecision,
			CreatedAt:                pullRequest.CreatedAt,
			UpdatedAt:                pullRequest.UpdatedAt,
			ClosedAt:                 pullRequest.ClosedAt,
			MergedAt:                 pullRequest.MergedAt,
			MergedBy:                 pullRequest.MergedBy.Login,
			Draft:                    pullRequest.IsDraft,
			Milestone:                pullRequest.Milestone.Title,
			TotalCommentCount:        pullRequest.TotalCommentsCount,
			AssociatedLabels:         make(map[string]bool),
			AssociatedProjectNumbers: make(map[int]bool),
		}

		for _, assignee := range pullRequest.Assignees.Nodes {
			pr.Assignees = append(pr.Assignees, assignee.Login)
		}

		for _, project := range pullRequest.ProjectItems.Nodes {
			pr.AssociatedProjectNumbers[project.Project.Number] = true
		}

		for _, issue := range pullRequest.ClosingIssuesReferences.Nodes {
			pr.ClosingIssues = append(pr.ClosingIssues, ClosingIssue{
				NodeID: issue.ID,
				Number: issue.Number,
			})
		}

		for _, label := range pullRequest.Labels.Nodes {
			pr.AssociatedLabels[label.Name] = true
		}

		for _, review := range pullRequest.Reviews.Nodes {
			// We're only interested in `APPROVED`, `CHANGES_REQUESTED`, and `DISMISSED` states.
			if review.State == string(githubv4.PullRequestReviewStateCommented) || review.State == string(githubv4.PullRequestReviewStatePending) {
				continue
			}
			pr.TotalReviewCount++
			pr.ReviewCommentCount += review.Comments.TotalCount

			// Only add filtered review count if `reviewers` filter was provided
			if _, ok := reviewers[strings.ToLower(review.Author.Login)]; ok {
				pr.FilteredReviewCount++
				pr.FilteredReviewCommentCount += review.Comments.TotalCount
			}
		}

		result = append(result, pr)
	}

	return result
}
