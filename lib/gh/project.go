package gh

import (
	"strconv"
)

type Project struct {
	Owner  string
	Number int
	Token

	*ProjectDetails
}

func NewProject(owner string, number int, token string) Project {
	p := Project{
		Owner:  owner,
		Number: number,
		Token: Token{
			Token: nil,
		},
	}

	if token != "" {
		p.Token.Token = &token
	}

	return p
}

type ProjectDetails struct {
	ID     string
	Fields []struct {
		ID      string
		Name    string
		Options []struct {
			ID   string
			Name string
		}
	}
	FieldIDs                map[string]string
	StatusIDs               map[string]string
	FieldTypes              map[string]ItemValueType     // field name -> type
	SingleSelectOptionIDs   map[string]map[string]string // field name -> option name -> option ID
	SingleSelectOptionNames map[string]map[string]string // field name -> option ID -> option name
}

type ProjectDetailsResult struct {
	Data struct {
		Organization struct {
			ProjectV2 struct {
				ID     string `json:"id"`
				Fields struct {
					Nodes []struct {
						ID      string `json:"id"`
						Name    string `json:"name"`
						Options []struct {
							ID   string `json:"id"`
							Name string `json:"name"`
						} `json:"options"`
					} `json:"nodes"`
				} `json:"fields"`
			} `json:"projectV2"`
		} `json:"organization"`
	} `json:"data"`
}

func (p *Project) LoadDetails() error {
	q := `query=
        query($org: String!, $number: Int!) {
            organization(login: $org){
                projectV2(number: $number) {
                    id
                    fields(first:100) {
                        nodes {
                            ... on ProjectV2Field {
                                id
                                name
                            }
                            ... on ProjectV2SingleSelectField {
                                id
                                name
                                options {
                                    id
                                    name
                                }
                            }
                            ... on ProjectV2IterationField {
                                id
                                name
                            }
                        }
                    }
                }
            }
        }
    `

	params := [][]string{
		{"-f", "org=" + p.Owner},
		{"-F", "number=" + strconv.Itoa(p.Number)},
	}

	var result ProjectDetailsResult
	if err := p.GraphQLQueryUnmarshal(q, params, &result); err != nil {
		return err
	}

	project := ProjectDetails{
		ID:                      result.Data.Organization.ProjectV2.ID,
		FieldIDs:                map[string]string{},
		StatusIDs:               map[string]string{},
		FieldTypes:              map[string]ItemValueType{},
		SingleSelectOptionIDs:   map[string]map[string]string{},
		SingleSelectOptionNames: map[string]map[string]string{},
	}

	for _, f := range result.Data.Organization.ProjectV2.Fields.Nodes {
		if f.ID == "" {
			continue // field type not covered by the query fragments
		}

		field := struct {
			ID      string
			Name    string
			Options []struct {
				ID   string
				Name string
			}
		}{
			ID:   f.ID,
			Name: f.Name,
		}

		if len(f.Options) > 0 {
			// This is a single-select field
			project.FieldTypes[f.Name] = ItemValueTypeSingleSelect
			project.SingleSelectOptionIDs[f.Name] = map[string]string{}
			project.SingleSelectOptionNames[f.Name] = map[string]string{}
			for _, s := range f.Options {
				field.Options = append(field.Options, struct {
					ID   string
					Name string
				}{
					ID:   s.ID,
					Name: s.Name,
				})
				project.SingleSelectOptionIDs[f.Name][s.Name] = s.ID
				project.SingleSelectOptionNames[f.Name][s.ID] = s.Name
			}
		} else {
			// Default to text for regular fields — number/date are indistinguishable
			// from the current query but UpdateItem handles them by type
			project.FieldTypes[f.Name] = ItemValueTypeText
		}

		project.Fields = append(project.Fields, field)
	}

	for _, f := range project.Fields {
		project.FieldIDs[f.Name] = f.ID

		if f.Name == "Status" {
			for _, s := range f.Options {
				project.StatusIDs[s.Name] = s.ID
			}
		}
	}

	p.ProjectDetails = &project
	return nil
}
