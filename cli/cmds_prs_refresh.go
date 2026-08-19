package cli

import (
	"fmt"
	"strings"
	"time"

	c "github.com/gookit/color"
	"github.com/katbyte/ghp-sync/lib/gh"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// prRefreshDefaultFields are the fields refreshed by default for closed/merged PRs. Fields needing
// data we don't fetch here (review counts, waiting days) are excluded so we don't overwrite real
// values with zeros; use --pr-populate-fields to override.
var prRefreshDefaultFields = []string{"Status", "PR#", "User", "Open Days", "Created At", "Closed At", "Merged At", "Merged By", "Reviewed By", "Approved By"}

// prRefreshOpenFields are the fields safe to refresh on open PRs with --include-open: the REST
// lookup can't see the review decision, so Status would incorrectly knock "Approved" PRs back
// to "Waiting", and waiting/count data isn't available at all.
var prRefreshOpenFields = map[string]bool{"PR#": true, "User": true, "Created At": true, "Open Days": true, "Reviewed By": true, "Approved By": true}

// CmdPRsRefresh walks the project board itself and refreshes fields on PR items that are now
// closed or merged, rather than syncing PRs from a repo. This catches PRs that were added while
// open but have since closed, without crawling the repo's full PR history.
func CmdPRsRefresh(_ *cobra.Command, _ []string) error {
	f := GetFlags()
	includeOpen := viper.GetBool("include-open")
	p := gh.NewProject(f.ProjectOwner, f.ProjectNumber, f.Token)

	c.Printf("Looking up project details for <green>%s</>/<lightGreen>%d</>...\n", f.ProjectOwner, f.ProjectNumber)
	if err := p.LoadDetails(); err != nil {
		return fmt.Errorf("loading project details: %w", err)
	}
	c.Printf("  ID: <magenta>%s</>\n", p.ID)

	// resolve which fields to refresh: explicit populate list, or the refresh defaults minus skips
	fieldNames := f.PRPopulateFields
	if len(fieldNames) == 0 {
		skip := map[string]bool{}
		for _, name := range f.PRSkipFields {
			skip[name] = true
		}
		for _, name := range prRefreshDefaultFields {
			if !skip[name] {
				fieldNames = append(fieldNames, name)
			}
		}
	}

	var prFields []string
	for _, fieldName := range fieldNames {
		if _, known := PRFields[fieldName]; !known {
			return fmt.Errorf("unknown pr field %q, available: %s", fieldName, strings.Join(prFieldNames(), ", "))
		}
		if _, ok := p.FieldIDs[fieldName]; !ok {
			if len(f.PRPopulateFields) > 0 || f.Strict {
				return fmt.Errorf("pr field %q not found in project", fieldName)
			}
			c.Printf("<yellow>WARNING:</> pr field <lightBlue>%q</> not found in project, skipping\n", fieldName)
			continue
		}
		prFields = append(prFields, fieldName)
	}

	needReviews := false
	for _, fieldName := range prFields {
		if fieldName == "Reviewed By" || fieldName == "Approved By" {
			needReviews = true
		}
	}

	// optional repo filter
	repoFilter := map[string]bool{}
	for _, repo := range f.Repos {
		repoFilter[strings.ToLower(repo)] = true
	}

	c.Printf("\n<white>Configuration:</>\n")
	c.Printf("  <lightBlue>pr fields</>:    <lightGreen>%s</>\n", strings.Join(prFields, ", "))
	if len(f.Repos) > 0 {
		c.Printf("  <lightBlue>repos</>:        <cyan>%s</>\n", strings.Join(f.Repos, ", "))
	}
	if includeOpen {
		c.Printf("  <lightBlue>include open</>: <yellow>yes</>\n")
	}
	if f.DryRun {
		c.Printf("  <lightBlue>dry run</>:      <yellow>yes</>\n")
	}
	fmt.Println()

	c.Printf("Getting project items.. ")
	items, err := p.GetItems()
	if err != nil {
		return fmt.Errorf("getting project items: %w", err)
	}
	c.Printf("<yellow>%d</>\n\n", len(items))

	repos := map[string]*gh.Repo{}
	refreshed, skippedOpen := 0, 0
	byStatus := map[string][]int{}

	for i, item := range items {
		if item.URL == "" {
			continue // draft or redacted item
		}

		owner, name, typ, number, parseErr := gh.ParseGitHubURL(item.URL)
		if parseErr != nil || typ != "pull" {
			continue
		}

		fullName := strings.ToLower(owner + "/" + name)
		if len(repoFilter) > 0 && !repoFilter[fullName] {
			continue
		}

		c.Printf("<white>%d</><gray>/%d</> <blue>%s</>/<lightBlue>%s</>#<lightCyan>%d</> <darkGray>%s</> ", i+1, len(items), owner, name, number, item.URL)

		r, ok := repos[fullName]
		if !ok {
			r, err = gh.NewRepo(owner+"/"+name, f.Token)
			if err != nil {
				return fmt.Errorf("creating repo %s/%s: %w", owner, name, err)
			}
			repos[fullName] = r
		}

		rpr, prErr := r.GetPullRequest(number)
		if prErr != nil {
			c.Printf("<red>ERROR!!</> %s\n", prErr)
			continue
		}

		isOpen := rpr.GetState() == "open"
		if isOpen && !includeOpen {
			skippedOpen++
			c.Printf("<gray>open, skipping</>\n")
			continue
		}

		state := "CLOSED"
		statusText := "Closed"
		switch {
		case isOpen:
			state = "OPEN"
			statusText = "Open"
		case rpr.GetMerged():
			state = "MERGED"
			statusText = "Merged"
		}

		pr := gh.PullRequest{
			NodeID:    rpr.GetNodeID(),
			Author:    rpr.GetUser().GetLogin(),
			Number:    rpr.GetNumber(),
			Title:     rpr.GetTitle(),
			State:     state,
			CreatedAt: rpr.GetCreatedAt().Time,
			UpdatedAt: rpr.GetUpdatedAt().Time,
			ClosedAt:  rpr.GetClosedAt().Time,
			MergedAt:  rpr.GetMergedAt().Time,
			MergedBy:  rpr.GetMergedBy().GetLogin(),
			Draft:     rpr.GetDraft(),
			Milestone: rpr.GetMilestone().GetTitle(),
		}

		if needReviews {
			reviews, reviewsErr := r.GetPullRequestReviews(number)
			if reviewsErr != nil {
				c.Printf("<yellow>WARNING: getting reviews:</> %s ", reviewsErr)
			}
			reviewedBy := map[string]bool{}
			approvedBy := map[string]bool{}
			for _, review := range reviews {
				login := review.GetUser().GetLogin()
				switch review.GetState() {
				case "APPROVED":
					if !approvedBy[login] {
						approvedBy[login] = true
						pr.ApprovedBy = append(pr.ApprovedBy, login)
					}
				case "CHANGES_REQUESTED", "COMMENTED", "DISMISSED":
					if !reviewedBy[login] {
						reviewedBy[login] = true
						pr.ReviewedBy = append(pr.ReviewedBy, login)
					}
				}
			}
		}

		switch statusText {
		case "Merged":
			c.Printf("<green>Merged</> by <yellow>%s</> ", pr.MergedBy)
		case "Open":
			c.Printf("<yellow>Open</> ")
		default:
			c.Printf("<darkred>Closed</> ")
		}

		daysOpen := int(pr.ClosedAt.Sub(pr.CreatedAt) / (time.Hour * 24))
		itemFields := prFields
		if isOpen {
			daysOpen = int(time.Since(pr.CreatedAt) / (time.Hour * 24))
			itemFields = nil
			for _, fieldName := range prFields {
				if prRefreshOpenFields[fieldName] {
					itemFields = append(itemFields, fieldName)
				}
			}
		}

		fieldCtx := PRFieldContext{
			PR:       &pr,
			Project:  p,
			DaysOpen: daysOpen,
			Status:   statusText,
		}

		var fields []gh.ProjectItemField
		for _, fieldName := range itemFields {
			value := PRFields[fieldName].ComputeFn(fieldCtx)
			if value == nil {
				continue
			}

			fields = append(fields, gh.ProjectItemField{
				Name:    strings.ToLower(strings.NewReplacer(" ", "_", "#", "").Replace(fieldName)),
				FieldID: p.FieldIDs[fieldName],
				Type:    PRFields[fieldName].Type,
				Value:   value,
			})
		}

		if f.DryRun {
			c.Printf("<yellow>[dry-run: would update %d fields]</>\n", len(fields))
			for _, fieldName := range itemFields {
				if value := PRFields[fieldName].ComputeFn(fieldCtx); value != nil {
					c.Printf("    <gray>[dry-run]</> <lightBlue>%s</> = <white>%v</>\n", fieldName, displayFieldValue(p, fieldName, PRFields[fieldName].Type, value))
				}
			}
		} else {
			if err = p.UpdateItem(item.ID, fields); err != nil {
				c.Printf("<red>ERROR!!</> %s\n", err)
				continue
			}
			c.Printf("<lightGreen>✓ %d fields</>\n", len(fields))
		}

		refreshed++
		byStatus[statusText] = append(byStatus[statusText], pr.Number)
	}

	fmt.Println()
	for k := range byStatus {
		c.Printf("<cyan>%s</><gray>x%d -</> %s\n", k, len(byStatus[k]), strings.Trim(strings.ReplaceAll(fmt.Sprint(byStatus[k]), " ", ","), "[]"))
	}
	c.Printf("refreshed <lightGreen>%d</> items, skipped <yellow>%d</> still open\n", refreshed, skippedOpen)

	return nil
}
