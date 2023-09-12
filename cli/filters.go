package cli

import (
	"fmt"
	"github.com/google/go-github/v45/github"
	c "github.com/gookit/color" //nolint:misspell
	"strings"
)

type Filter struct {
	Name  string
	Issue func(options github.Issue) (bool, error)
}

func (f FlagData) GetFilters() []Filter {
	var filters []Filter

	// should these return errors
	if f := GetFilterForLabels(f.Labels); f != nil {
		filters = append(filters, *f)
	}

	fmt.Println()

	return filters
}

func GetFilterForLabels(labels []string) *Filter {
	if len(labels) == 0 {
		return nil
	}

	filterLabelMap := map[string]bool{}
	for _, l := range labels {
		filterLabelMap[strings.TrimPrefix(l, "-")] = strings.HasPrefix(l, "-")
	}

	c.Printf("  labels:  <blue>%s</>\n", strings.Join(labels, "</>,<blue>"))

	//	found := false
	return &Filter{
		Name: "labels" + "and",
		Issue: func(issue github.Issue) (bool, error) {
			labelMap := map[string]bool{}
			for _, l := range issue.Labels {
				labelMap[l.GetName()] = true
			}

			c.Printf("    labels: ")

			andFail := false

			// for each filter label see if it exists
			for filterLabel, negate := range filterLabelMap {
				_, found := labelMap[filterLabel]

				// nolint:gocritic
				if found && !negate {
					c.Printf(" <green>%s</>", filterLabel)
				} else if found && negate {
					andFail = true
					c.Printf(" <red>-%s</>", filterLabel)
				} else if negate {
					c.Printf(" <green>-%s</>", filterLabel)
				} else {
					andFail = true
					c.Printf(" <red>%s</>", filterLabel)
				}
			}
			fmt.Println()

			return !andFail, nil

		},
	}
}
