package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/katbyte/ghp-sync/lib/gh"
)

// PRFieldContext holds all the data needed to compute field values
type PRFieldContext struct {
	PR          *gh.PullRequest
	Project     gh.Project
	DaysOpen    int
	DaysWaiting int
	Status      string // The computed status text (e.g., "In Progress", "Approved")
}

// PRFieldDef defines a field that can be populated on a GitHub Project item
type PRFieldDef struct {
	Type      gh.ItemValueType // Field type for GraphQL mutation
	ComputeFn func(ctx PRFieldContext) any
}

// displayFieldValue renders a computed field value for humans, translating single select
// option IDs back to their option names
func displayFieldValue(p gh.Project, fieldName string, t gh.ItemValueType, value any) any {
	if t == gh.ItemValueTypeSingleSelect {
		if name, ok := p.SingleSelectOptionNames[fieldName][fmt.Sprint(value)]; ok {
			return name
		}
	}
	return value
}

// prFieldNames returns the sorted names of all registered PR fields
func prFieldNames() []string {
	names := make([]string, 0, len(PRFields))
	for name := range PRFields {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// PRFields is the registry of all available PR fields, keyed by field name (matches GitHub Project field name)
var PRFields = map[string]PRFieldDef{
	"PR#": {
		Type: gh.ItemValueTypeNumber,
		ComputeFn: func(ctx PRFieldContext) any {
			return ctx.PR.Number
		},
	},
	"Status": {
		Type: gh.ItemValueTypeSingleSelect,
		ComputeFn: func(ctx PRFieldContext) any {
			id, ok := ctx.Project.StatusIDs[ctx.Status]
			if !ok || id == "" {
				fmt.Printf("WARNING: status %q not found in project\n", ctx.Status)
				return nil
			}
			return id
		},
	},
	"User": {
		Type: gh.ItemValueTypeText,
		ComputeFn: func(ctx PRFieldContext) any {
			return ctx.PR.Author
		},
	},
	"Merged By": {
		Type: gh.ItemValueTypeText,
		ComputeFn: func(ctx PRFieldContext) any {
			if ctx.PR.MergedBy == "" {
				return nil // not merged
			}
			return ctx.PR.MergedBy
		},
	},
	"Open Days": {
		Type: gh.ItemValueTypeNumber,
		ComputeFn: func(ctx PRFieldContext) any {
			return ctx.DaysOpen
		},
	},
	"Waiting Days": {
		Type: gh.ItemValueTypeNumber,
		ComputeFn: func(ctx PRFieldContext) any {
			return ctx.DaysWaiting
		},
	},
	"Comment Count": {
		Type: gh.ItemValueTypeNumber,
		ComputeFn: func(ctx PRFieldContext) any {
			return ctx.PR.TotalCommentCount
		},
	},
	"Review Count": {
		Type: gh.ItemValueTypeNumber,
		ComputeFn: func(ctx PRFieldContext) any {
			return ctx.PR.TotalReviewCount
		},
	},
	"Review Comment Count": {
		Type: gh.ItemValueTypeNumber,
		ComputeFn: func(ctx PRFieldContext) any {
			return ctx.PR.ReviewCommentCount
		},
	},
	"Created At": {
		Type: gh.ItemValueTypeDate,
		ComputeFn: func(ctx PRFieldContext) any {
			return ctx.PR.CreatedAt.Format(time.RFC3339)
		},
	},
	"Closed At": {
		Type: gh.ItemValueTypeDate,
		ComputeFn: func(ctx PRFieldContext) any {
			if strings.EqualFold(ctx.PR.State, "open") {
				return nil // Don't set for open PRs
			}
			return ctx.PR.ClosedAt.Format(time.RFC3339)
		},
	},
	"Merged At": {
		Type: gh.ItemValueTypeDate,
		ComputeFn: func(ctx PRFieldContext) any {
			if ctx.PR.MergedAt.IsZero() {
				return nil // not merged
			}
			return ctx.PR.MergedAt.Format(time.RFC3339)
		},
	},
	"Filtered Review Count": {
		Type: gh.ItemValueTypeNumber,
		ComputeFn: func(ctx PRFieldContext) any {
			if ctx.PR.FilteredReviewCount == 0 {
				return nil
			}
			return ctx.PR.FilteredReviewCount
		},
	},
	"Filtered Review Comment Count": {
		Type: gh.ItemValueTypeNumber,
		ComputeFn: func(ctx PRFieldContext) any {
			if ctx.PR.FilteredReviewCount == 0 {
				return nil
			}
			return ctx.PR.FilteredReviewCommentCount
		},
	},
}
