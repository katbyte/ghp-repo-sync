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
var prRefreshDefaultFields = []string{"Status", "PR#", "User", "Open Days", "Created At", "Closed At", "Merged At", "Merged By", "Reviewed By", "Approved By", "Changes Requested By", "CI", "Mergeable"}

// prRefreshOpenFields are the fields safe to refresh on open PRs with --include-open: the REST
// lookup can't see the review decision, so Status would incorrectly knock "Approved" PRs back
// to "Waiting", and waiting/count data isn't available at all. CI/Mergeable are fetched
// separately via GraphQL when needed.
var prRefreshOpenFields = map[string]bool{"PR#": true, "User": true, "Created At": true, "Open Days": true, "Reviewed By": true, "Approved By": true, "Changes Requested By": true, "CI": true, "Mergeable": true}

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

	needReviews, needChangesRequested, needMergeable := false, false, false
	for _, fieldName := range prFields {
		if fieldName == "Reviewed By" || fieldName == "Approved By" || fieldName == "Changes Requested By" {
			needReviews = true
		}
		if fieldName == "Changes Requested By" {
			needChangesRequested = true
		}
		if fieldName == "CI" || fieldName == "Mergeable" {
			needMergeable = true
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
	refreshed, unchanged, skippedOpen := 0, 0, 0
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

		if needMergeable && isOpen {
			mergeable, checkState, msErr := r.GetPullRequestMergeStatus(number)
			if msErr != nil {
				c.Printf("<yellow>WARNING: getting merge status:</> %s ", msErr)
			}
			pr.Mergeable = mergeable
			pr.CheckState = checkState
		}

		if needReviews {
			reviews, reviewsErr := r.GetPullRequestReviews(number)
			if reviewsErr != nil {
				c.Printf("<yellow>WARNING: getting reviews:</> %s ", reviewsErr)
			}
			reviewedBy := map[string]bool{}
			approvedBy := map[string]bool{}
			type changeRequest struct {
				login    string
				reviewID int64
			}
			var changeRequests []changeRequest
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
					if review.GetState() == "CHANGES_REQUESTED" && needChangesRequested {
						changeRequests = append(changeRequests, changeRequest{login: login, reviewID: review.GetID()})
					}
				}
			}

			// the review list api doesn't include per-review comment counts, so fetch the PR's
			// review comments (each tagged with its review id) and tally them per change request
			if len(changeRequests) > 0 {
				commentsPerReview := map[int64]int{}
				comments, commentsErr := r.GetPullRequestReviewComments(number)
				if commentsErr != nil {
					c.Printf("<yellow>WARNING: getting review comments:</> %s ", commentsErr)
				}
				for _, comment := range comments {
					commentsPerReview[comment.GetPullRequestReviewID()]++
				}

				requestedBy := map[string]int{} // login -> index into pr.ChangesRequestedBy
				for _, cr := range changeRequests {
					if idx, ok := requestedBy[cr.login]; ok {
						pr.ChangesRequestedBy[idx].Requests++
						pr.ChangesRequestedBy[idx].Comments += commentsPerReview[cr.reviewID]
					} else {
						requestedBy[cr.login] = len(pr.ChangesRequestedBy)
						pr.ChangesRequestedBy = append(pr.ChangesRequestedBy, gh.ReviewerCommentCount{Login: cr.login, Requests: 1, Comments: commentsPerReview[cr.reviewID]})
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

		type fieldChange struct {
			name     string
			from, to any
		}

		var fields []gh.ProjectItemField
		var changes []fieldChange
		for _, fieldName := range itemFields {
			fieldType := PRFields[fieldName].Type
			value := PRFields[fieldName].ComputeFn(fieldCtx)
			if value == nil {
				continue
			}

			current, exists := item.FieldValues[fieldName]
			if prFieldValueUnchanged(fieldType, current, exists, value) {
				continue
			}

			fields = append(fields, gh.ProjectItemField{
				Name:    strings.ToLower(strings.NewReplacer(" ", "_", "#", "").Replace(fieldName)),
				FieldID: p.FieldIDs[fieldName],
				Type:    fieldType,
				Value:   value,
			})

			from := any("(unset)")
			if exists {
				from = displayFieldValue(p, fieldName, fieldType, current.Value)
			}
			to := displayFieldValue(p, fieldName, fieldType, value)
			if fieldType == gh.ItemValueTypeDate {
				to = trimToDay(fmt.Sprint(to))
			}
			changes = append(changes, fieldChange{name: fieldName, from: from, to: to})
		}

		if len(fields) == 0 {
			c.Printf("<gray>up to date</>\n")
			unchanged++
			continue
		}

		if f.DryRun {
			c.Printf("<yellow>[dry-run: would update %d fields]</>\n", len(fields))
			for _, ch := range changes {
				c.Printf("    <gray>[dry-run]</> <lightBlue>%s</>: <darkGray>%v</> <gray>-></> <white>%v</>\n", ch.name, ch.from, ch.to)
			}
		} else {
			if err = p.UpdateItem(item.ID, fields); err != nil {
				c.Printf("<red>ERROR!!</> %s\n", err)
				continue
			}
			c.Printf("<lightGreen>✓ %d fields</>\n", len(fields))
			for _, ch := range changes {
				c.Printf("    <lightBlue>%s</>: <darkGray>%v</> <gray>-></> <white>%v</>\n", ch.name, ch.from, ch.to)
			}
		}

		refreshed++
		byStatus[statusText] = append(byStatus[statusText], pr.Number)
	}

	fmt.Println()
	for k := range byStatus {
		c.Printf("<cyan>%s</><gray>x%d -</> %s\n", k, len(byStatus[k]), strings.Trim(strings.ReplaceAll(fmt.Sprint(byStatus[k]), " ", ","), "[]"))
	}
	c.Printf("refreshed <lightGreen>%d</> items, <gray>%d</> up to date, skipped <yellow>%d</> still open\n", refreshed, unchanged, skippedOpen)

	return nil
}

// prFieldValueUnchanged reports whether the computed value matches the item's current board value.
// Date fields are compared on the day only, as the project stores dates without a time component.
func prFieldValueUnchanged(t gh.ItemValueType, current gh.ProjectItemFieldValue, exists bool, value any) bool {
	switch t {
	case gh.ItemValueTypeNumber:
		if !exists {
			return false
		}
		cur, ok := current.Value.(float64)
		if !ok {
			return false
		}
		switch v := value.(type) {
		case int:
			return cur == float64(v)
		case float64:
			return cur == v
		}
		return false
	case gh.ItemValueTypeDate:
		cur := ""
		if exists {
			cur, _ = current.Value.(string)
		}
		return trimToDay(cur) == trimToDay(fmt.Sprint(value))
	default: // text and single select option IDs compare as strings; unset counts as empty
		cur := ""
		if exists {
			cur, _ = current.Value.(string)
		}
		return cur == fmt.Sprint(value)
	}
}

func trimToDay(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}
