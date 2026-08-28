package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	c "github.com/gookit/color"
	"github.com/katbyte/ghp-sync/lib/gh"
	"github.com/spf13/cobra"
)

func CmdPRs(_ *cobra.Command, _ []string) error {
	f := GetFlags()

	var mergedSince *time.Time
	if f.Filters.MergedSince != "" {
		t, err := time.Parse("2006-01-02", f.Filters.MergedSince)
		if err != nil {
			if t, err = time.Parse(time.RFC3339, f.Filters.MergedSince); err != nil {
				return fmt.Errorf("parsing merged-since %q (expected YYYY-MM-DD or RFC3339): %w", f.Filters.MergedSince, err)
			}
		}
		mergedSince = &t

		hasMerged := false
		for _, state := range f.Filters.States {
			if strings.EqualFold(state, "MERGED") {
				hasMerged = true
			}
		}
		if !hasMerged {
			return fmt.Errorf("--merged-since requires MERGED in --pr-states (currently %s)", strings.Join(f.Filters.States, ","))
		}
	}

	p := gh.NewProject(f.ProjectOwner, f.ProjectNumber, f.Token)

	c.Printf("Looking up project details for <green>%s</>/<lightGreen>%d</>...\n", f.ProjectOwner, f.ProjectNumber)
	err := p.LoadDetails()
	if err != nil {
		return fmt.Errorf("loading project details: %w", err)
	}
	c.Printf("  ID: <magenta>%s</>\n", p.ID)

	// todo we can probably remove this? its just for printing the fields (we do this multiple times so maybe just a helper)
	for _, field := range p.Fields {
		c.Printf("    <lightBlue>%s</> <> <lightCyan>%s</>\n", field.Name, field.ID)

		if field.Name == "Status" {
			for _, s := range field.Options {
				c.Printf("      <blue>%s</> <> <cyan>%s</>\n", s.Name, s.ID)
			}
		}
	}
	fmt.Println()

	// validate pr fields against the project up front, dropping (or in strict mode erroring on)
	// ones the project doesn't have so we only warn once rather than for every pr
	var prFields []string
	for _, fieldName := range f.PRFields {
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
	f.PRFields = prFields
	fmt.Println()

	needMergeStatus := false
	for _, fieldName := range f.PRFields {
		if fieldName == "Mergeable" {
			needMergeStatus = true
		}
	}

	// Print config summary
	c.Printf("<white>Configuration:</>\n")
	c.Printf("  <lightBlue>repos</>:        ")
	for i, repo := range f.Repos {
		if i > 0 {
			c.Printf("<gray>,</> ")
		}
		c.Printf("<cyan>%s</>", repo)
	}
	c.Printf("\n")
	c.Printf("  <lightBlue>pr states</>:    <green>%s</>\n", strings.Join(f.Filters.States, ", "))
	if len(f.Filters.Authors) > 0 {
		c.Printf("  <lightBlue>authors</>:      <yellow>%s</>\n", strings.Join(f.Filters.Authors, ", "))
	}
	if len(f.Filters.Assignees) > 0 {
		c.Printf("  <lightBlue>assignees</>:    <yellow>%s</>\n", strings.Join(f.Filters.Assignees, ", "))
	}
	if len(f.Filters.MergedBy) > 0 {
		c.Printf("  <lightBlue>merged by</>:    <yellow>%s</>\n", strings.Join(f.Filters.MergedBy, ", "))
	}
	if mergedSince != nil {
		c.Printf("  <lightBlue>merged since</>: <yellow>%s</>\n", mergedSince.Format("2006-01-02"))
	}
	if f.Filters.FiltersOnly {
		c.Printf("  <lightBlue>filters only</>: <yellow>yes (prs already in project are not auto-included)</>\n")
	}
	if len(f.Filters.Reviewers) > 0 {
		c.Printf("  <lightBlue>reviewers</>:    <yellow>%s</>\n", strings.Join(f.Filters.Reviewers, ", "))
	}
	if len(f.Filters.LabelsOr) > 0 {
		c.Printf("  <lightBlue>labels (or)</>:  <yellow>%s</>\n", strings.Join(f.Filters.LabelsOr, ", "))
	}
	if len(f.Filters.LabelsAnd) > 0 {
		c.Printf("  <lightBlue>labels (and)</>: <yellow>%s</>\n", strings.Join(f.Filters.LabelsAnd, ", "))
	}
	c.Printf("  <lightBlue>pr fields</>:    <lightGreen>%s</>\n", strings.Join(f.PRFields, ", "))
	if len(f.SyncLinkedIssueFields) > 0 {
		c.Printf("  <lightBlue>issue sync</>:   <magenta>%s</>\n", strings.Join(f.SyncLinkedIssueFields, ", "))
	} else {
		c.Printf("  <lightBlue>issue sync</>:   <gray>disabled</>\n")
	}
	if f.ItemLimit > 0 {
		c.Printf("  <lightBlue>item limit</>:   <yellow>%d</>\n", f.ItemLimit)
	}
	if f.DryRun {
		c.Printf("  <lightBlue>dry run</>:      <yellow>yes</>\n")
	}
	fmt.Println()

	// for each repo, get all prs, and add to project
	for _, repo := range f.Repos {
		r, err := gh.NewRepo(repo, f.Token)
		if err != nil {
			return fmt.Errorf("creating repo %s: %w", repo, err)
		}

		limitMsg := ""
		if f.ItemLimit != 0 {
			limitMsg = " limited to: <yellow>" + strconv.Itoa(f.ItemLimit) + "</> items"
		}

		// get all pull requests
		c.Printf("Retrieving all prs for <white>%s</>/<cyan>%s</> with states <green>%s</>%s. Loaded ", r.Owner, r.Name, f.Filters.States, limitMsg)
		prs, err := r.GetAllPullRequestsGQL(f.Filters.States, f.Filters.Reviewers, f.ItemLimit, mergedSince, func(i int) {
			fmt.Printf("%d ", i)
		})
		if err != nil {
			return fmt.Errorf("getting PRs for %s/%s: %w", r.Owner, r.Name, err)
		}
		c.Printf("<yellow>%d</> items\n", len(*prs))
		prs, matchReasons := FilterByFlags(f, prs)

		byStatus := map[string][]int{}

		for i, pr := range *prs {
			prNode := pr.NodeID

			matched := ""
			if why, ok := matchReasons[pr.Number]; ok {
				matched = " <yellow>[" + why + "]</>"
			}
			c.Printf("<white>%d</><gray>/%d</> Syncing pr <lightCyan>%d</> (<cyan>%s</>)%s to project.. <darkGray>%s</>\n  ", i+1, len(*prs), pr.Number, prNode, matched, r.PrURL(pr.Number))

			var iid *string
			if !f.DryRun {
				iid, err = p.AddItem(prNode)
				if err != nil {
					c.Printf("\n\n <red>ERROR!!</> %s", err)
					continue
				}
				c.Printf("<magenta>%s</>", *iid)
			} else {
				c.Printf("<yellow>[dry-run]</>")
			}

			daysOpen := int(time.Since(pr.CreatedAt) / (time.Hour * 24))
			daysWaiting := 0

			var statusText string
			switch {
			case strings.EqualFold(pr.State, "merged"):
				statusText = "Merged"
				daysOpen = int(pr.ClosedAt.Sub(pr.CreatedAt) / (time.Hour * 24))
				c.Printf("  <green>Merged</>\n")
			case pr.ReviewDecision == "APPROVED": // TODO if approved make sure it stays approved
				statusText = "Approved"
				c.Printf("  <blue>Approved</> <gray>(reviews)</>\n")
			case strings.EqualFold(pr.State, "closed"): // We filter by open PRs so pr.State should never be `closed`?
				statusText = "Closed"
				daysOpen = int(pr.ClosedAt.Sub(pr.CreatedAt) / (time.Hour * 24))
				c.Printf("  <darkred>Closed</> <gray>(state)</>\n")

			case strings.EqualFold(pr.Milestone, "Blocked"):
				statusText = "Blocked"
				c.Printf("  <red>Blocked</> <gray>(milestone)</>\n")
			case pr.Draft:
				statusText = "In Progress"
				c.Printf("  <yellow>In Progress</> <gray>(draft)</>\n")
			case pr.State == "":
				statusText = "In Progress"
				c.Printf("  <yellow>In Progress</> <gray>(unknown state)</>\n")
			case pr.AssociatedLabels["waiting-response"]:
				statusText = "Waiting for Response"
				c.Printf("  <lightGreen>Waiting for Response</> <gray>(label)</>\n")
			default:
				statusText = "Waiting"
				c.Printf("  <green>Waiting for Review</> <gray>(default)</>")

				// calculate days waiting
				daysWaiting = daysOpen

				events, eventsErr := r.GetAllIssueEvents(pr.Number)
				if eventsErr != nil {
					return fmt.Errorf("getting events for PR %d: %w", pr.Number, eventsErr)
				}
				c.Printf(" with <magenta>%d</> events\n", len(*events))

				for _, t := range *events {
					// check for waiting response label removed
					if t.GetEvent() == "unlabeled" {
						if t.Label.GetName() == "waiting-response" {
							daysWaiting = int(time.Since(t.GetCreatedAt().Time) / (time.Hour * 24))
							break
						}
					}

					// check for blocked milestone removal
					if t.GetEvent() == "demilestoned" {
						if t.Milestone.GetTitle() == "Blocked" {
							daysWaiting = int(time.Since(t.GetCreatedAt().Time) / (time.Hour * 24))
							break
						}
					}
				}
			}

			byStatus[statusText] = append(byStatus[statusText], pr.Number)

			c.Printf("  open %d days, waiting %d days\n", daysOpen, daysWaiting)

			// github computes mergeability lazily and the bulk query doesn't wait for it;
			// re-query the pr (which also kicks off the computation) with retries so we
			// only stamp a ? when it truly never settles
			if needMergeStatus && strings.EqualFold(pr.State, "open") && pr.Mergeable == "UNKNOWN" {
				c.Printf("  <gray>mergeability unknown, waiting for github..</> ")
				if mergeable, checkState, msErr := r.GetPullRequestMergeStatus(pr.Number); msErr != nil {
					c.Printf("<yellow>WARNING:</> %s\n", msErr)
				} else {
					pr.Mergeable, pr.CheckState = mergeable, checkState
					c.Printf("<white>%s</>\n", mergeable)
				}
			}

			// Build field context for computing values
			fieldCtx := PRFieldContext{
				PR:          &pr,
				Project:     p,
				DaysOpen:    daysOpen,
				DaysWaiting: daysWaiting,
				Status:      statusText,
			}

			// Build fields dynamically from registry, f.PRFields was validated up front
			var fields []gh.ProjectItemField
			for _, fieldName := range f.PRFields {
				fieldID := p.FieldIDs[fieldName]
				fieldDef := PRFields[fieldName]
				value := fieldDef.ComputeFn(fieldCtx)
				if value == nil {
					continue // ComputeFn returned nil, skip this field
				}

				fields = append(fields, gh.ProjectItemField{
					Name:    strings.ToLower(strings.NewReplacer(" ", "_", "#", "").Replace(fieldName)),
					FieldID: fieldID,
					Type:    fieldDef.Type,
					Value:   value,
				})

				if f.DryRun {
					c.Printf("    <gray>[dry-run]</> <lightBlue>%s</> = <white>%v</>\n", fieldName, displayFieldValue(p, fieldName, fieldDef.Type, value))
				}
			}

			if !f.DryRun && iid != nil {
				err = p.UpdateItem(*iid, fields)
				if err != nil {
					c.Printf("<red>ERROR!!</> %s\n\n", err)
					continue
				}
			} else if f.DryRun {
				c.Printf(" <yellow>[dry-run: would update %d fields]</>", len(fields))
			}

			// Sync fields from linked issues if configured
			if len(f.SyncLinkedIssueFields) > 0 {
				if len(pr.ClosingIssues) == 0 {
					c.Printf("  <gray>🔗 linked issue sync: no closing issues referenced</>")
				} else {
					c.Printf("  <magenta>🔗</> linked issue sync (<cyan>%s</>)\n", strings.Join(f.SyncLinkedIssueFields, ", "))
					c.Printf("    closing issue(s): ")
					for i, ci := range pr.ClosingIssues {
						if i > 0 {
							c.Printf(", ")
						}
						c.Printf("<lightCyan>#%d</>", ci.Number)
					}
					c.Printf("\n")

					// Find which linked issues are in the project
					type foundIssue struct {
						NodeID string
						Number int
						ItemID string
					}
					var inProject []foundIssue
					for _, ci := range pr.ClosingIssues {
						c.Printf("    checking <lightCyan>#%d</> (<gray>%s</>).. ", ci.Number, ci.NodeID)
						itemID, lookupErr := p.HasItem(ci.NodeID)
						if lookupErr != nil {
							c.Printf("<red>ERROR!</> %s\n", lookupErr)
							continue
						}
						if itemID != nil {
							c.Printf("<green>✓ in project</> (<gray>%s</>)\n", *itemID)
							inProject = append(inProject, foundIssue{NodeID: ci.NodeID, Number: ci.Number, ItemID: *itemID})
						} else {
							c.Printf("<yellow>✗ not in project</>\n")
						}
					}

					switch {
					case len(inProject) == 0:
						c.Printf("    <yellow>⚠ no linked issues found in project, skipping field sync</>")
					case len(inProject) > 1:
						c.Printf("    <yellow>⚠ multiple linked issues in project (%d), skipping field sync</>", len(inProject))
					default:
						// Exactly one linked issue found — fetch its field values
						c.Printf("    reading fields from issue <lightCyan>#%d</>...\n", inProject[0].Number)
						issueFieldValues, lookupErr := p.GetItemFieldValuesByNodeID(inProject[0].NodeID, f.SyncLinkedIssueFields)
						if lookupErr != nil {
							c.Printf("    <red>ERROR!</> reading linked issue fields: %s", lookupErr)
						} else {
							var linkedFields []gh.ProjectItemField
							for _, fieldName := range f.SyncLinkedIssueFields {
								fv, ok := issueFieldValues[fieldName]
								if !ok {
									c.Printf("      <gray>%s: <empty></>\n", fieldName)
									continue
								}

								fieldID, hasField := p.FieldIDs[fieldName]
								if !hasField {
									c.Printf("      <yellow>%s: field not found in project, skipping</>\n", fieldName)
									continue
								}

								c.Printf("      <green>%s</>: <white>%v</> (<gray>%s</>)\n", fieldName, fv.Value, fv.Type)
								linkedFields = append(linkedFields, gh.ProjectItemField{
									Name:    "linked_" + strings.ToLower(strings.NewReplacer(" ", "_", "#", "").Replace(fieldName)),
									FieldID: fieldID,
									Type:    fv.Type,
									Value:   fv.Value,
								})
							}

							switch {
							case len(linkedFields) == 0:
								c.Printf("    <yellow>⚠ no field values to sync</>")
							case !f.DryRun && iid != nil:
								c.Printf("    syncing <lightGreen>%d</> field(s) to PR.. ", len(linkedFields))
								syncErr := p.UpdateItem(*iid, linkedFields)
								if syncErr != nil {
									c.Printf("<red>ERROR!</> %s", syncErr)
								} else {
									c.Printf("<green>✓ done</>")
								}
							case f.DryRun:
								c.Printf("    <yellow>[dry-run: would sync %d field(s)]</>", len(linkedFields))
							}
						}
					}
				}
				c.Printf("\n")
			}

			c.Printf("\n")

			// TODO remove closed PRs? move them to closed status?
		}

		// output
		for k := range byStatus { // todo sort? format as table? https://github.com/jedib0t/go-pretty
			c.Printf("<cyan>%s</><gray>x%d -</> %s\n", k, len(byStatus[k]), strings.Trim(strings.ReplaceAll(fmt.Sprint(byStatus[k]), " ", ","), "[]"))
		}
		c.Printf("\n")
	}
	return nil
}

// FilterByFlags returns the PRs matching the user filters along with a map of pr number ->
// why it matched. A nil map means no filters were active and everything was kept.
func FilterByFlags(f FlagData, prs *[]gh.PullRequest) (matched *[]gh.PullRequest, matchReasons map[int]string) {
	if len(f.Filters.Authors) == 0 && len(f.Filters.Assignees) == 0 && len(f.Filters.MergedBy) == 0 {
		return prs, nil
	}

	c.Printf(" filtering by authors: <yellow>%s:</>\n", f.Filters.Authors)
	c.Printf(" filtering by assignees: <yellow>%s:</>\n", f.Filters.Assignees)
	c.Printf(" filtering by merged by: <yellow>%s:</>\n", f.Filters.MergedBy)

	// map of users, lowercased as github logins are case-insensitive
	authorMap := map[string]bool{}
	for _, u := range f.Filters.Authors {
		authorMap[strings.ToLower(u)] = true
	}

	assigneeUserMap := map[string]bool{}
	for _, u := range f.Filters.Assignees {
		assigneeUserMap[strings.ToLower(u)] = true
	}

	mergedByMap := map[string]bool{}
	for _, u := range f.Filters.MergedBy {
		mergedByMap[strings.ToLower(u)] = true
	}

	var filteredPRs []gh.PullRequest
	reasons := map[int]string{}
	for _, pr := range *prs {
		reason := ""

		if !f.Filters.FiltersOnly && pr.AssociatedProjectNumbers[f.ProjectNumber] {
			reason = "already in project"
		}

		if reason == "" && authorMap[strings.ToLower(pr.Author)] {
			reason = "author " + pr.Author
		}

		if reason == "" {
			for _, a := range pr.Assignees {
				if assigneeUserMap[strings.ToLower(a)] {
					reason = "assignee " + a
					break
				}
			}
		}

		if reason == "" && mergedByMap[strings.ToLower(pr.MergedBy)] {
			reason = "merged by " + pr.MergedBy
		}

		if reason != "" {
			filteredPRs = append(filteredPRs, pr)
			reasons[pr.Number] = reason
		}
	}

	sort.Slice(filteredPRs, func(i, j int) bool {
		return filteredPRs[i].Number < filteredPRs[j].Number
	})

	c.Printf("  Found <lightBlue>%d</> filtered PRs: ", len(filteredPRs))
	for _, pr := range filteredPRs {
		c.Printf("<white>%d</>,", pr.Number)
	}
	c.Printf("\n\n")

	return &filteredPRs, reasons
}
