package cli

import (
	"fmt"
	"strings"

	"github.com/google/go-github/v89/github"
	c "github.com/gookit/color"
)

type Filter struct {
	Name  string
	Issue func(options github.Issue) (bool, error)
}

func (f FlagData) GetFilters() []Filter {
	var filters []Filter

	// should these return errors
	if f := GetFilterForAuthors(f.Filters.Authors); f != nil {
		filters = append(filters, *f)
	}

	if f := GetFilterForLabelsOr(f.Filters.LabelsOr); f != nil {
		filters = append(filters, *f)
	}

	if f := GetFilterForLabelsAnd(f.Filters.LabelsAnd); f != nil {
		filters = append(filters, *f)
	}

	fmt.Println()

	return filters
}

func GetFilterForLabelsOr(labels []string) *Filter {
	return GetFilterForLabels(labels, false)
}

func GetFilterForLabelsAnd(labels []string) *Filter {
	return GetFilterForLabels(labels, true)
}

func GetFilterForLabels(labels []string, and bool) *Filter {
	if len(labels) == 0 {
		return nil
	}

	filterLabelMap := map[string]bool{}
	for _, l := range labels {
		filterLabelMap[strings.TrimPrefix(l, "-")] = strings.HasPrefix(l, "-")
	}

	action := "or"
	actionAnd := false
	if and {
		action = "and"
		actionAnd = true
	}

	c.Printf("  labels %s:  <blue>%s</>\n", action, strings.Join(labels, "</>,<blue>"))

	return &Filter{
		Name: "labels " + action,
		Issue: func(issue github.Issue) (bool, error) {
			labelMap := map[string]bool{}
			for _, l := range issue.Labels {
				// todo check for emvy label name?
				labelMap[l.GetName()] = true // casing?
			}

			// and
			// for each label,

			if actionAnd {
				c.Printf("    labels all: ")
			} else {
				c.Printf("    labels any: ")
			}

			andFail := false
			orPass := false

			// for each filter label see if it exists
			for filterLabel, negate := range filterLabelMap {
				_, found := labelMap[filterLabel]

				//nolint:gocritic
				if found && !negate {
					orPass = true
					c.Printf(" <green>%s</>", filterLabel)
				} else if found && negate {
					andFail = true
					c.Printf(" <red>-%s</>", filterLabel)
				} else if negate {
					orPass = true
					c.Printf(" <green>-%s</>", filterLabel)
				} else {
					andFail = true
					c.Printf(" <red>%s</>", filterLabel)
				}
			}
			fmt.Println()

			if actionAnd {
				return !andFail, nil
			}

			return orPass, nil
		},
	}
}

func GetFilterForAuthors(authors []string) *Filter {
	if len(authors) == 0 {
		return nil
	}

	authorMap := map[string]bool{}
	for _, a := range authors {
		authorMap[strings.ToLower(a)] = true
	}

	c.Printf("  authors: <magenta>%s</>\n", strings.Join(authors, "</>,<magenta>"))

	return &Filter{
		Name: "authors",
		Issue: func(issue github.Issue) (bool, error) {
			author := issue.User.GetLogin()

			if _, ok := authorMap[strings.ToLower(author)]; ok {
				c.Printf("    author: <green>%s</>\n", author)
				return true, nil
			}
			c.Printf("    author: <red>%s</>\n", author)

			return false, nil
		},
	}
}
