package gh

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// HasItem checks whether a given content node (issue or PR) is already in this project.
// Returns the project item ID if found, nil if not found.
func (p *Project) HasItem(nodeID string) (*string, error) {
	if p.ProjectDetails == nil {
		return nil, errors.New("project details not loaded yet")
	}

	// Query the node directly and check its projectItems for our project
	q := `query=
		query($nodeId: ID!) {
			node(id: $nodeId) {
				... on Issue {
					projectItems(first: 50) {
						nodes {
							id
							project {
								id
							}
						}
					}
				}
				... on PullRequest {
					projectItems(first: 50) {
						nodes {
							id
							project {
								id
							}
						}
					}
				}
			}
		}
	`

	params := [][]string{
		{"-f", "nodeId=" + nodeID},
	}

	type hasItemResult struct {
		Data struct {
			Node struct {
				ProjectItems struct {
					Nodes []struct {
						ID      string `json:"id"`
						Project struct {
							ID string `json:"id"`
						} `json:"project"`
					} `json:"nodes"`
				} `json:"projectItems"`
			} `json:"node"`
		} `json:"data"`
	}

	var result hasItemResult
	if err := p.GraphQLQueryUnmarshal(q, params, &result); err != nil {
		return nil, fmt.Errorf("checking if item exists in project: %w", err)
	}

	for _, item := range result.Data.Node.ProjectItems.Nodes {
		if item.Project.ID == p.ID {
			return &item.ID, nil
		}
	}

	return nil, nil
}

func (p *Project) AddItem(nodeID string) (*string, error) {
	if p.ProjectDetails == nil {
		return nil, errors.New("project details not loaded yet")
	}

	q := `query=
        mutation($project:ID!, $pr:ID!) {
          addProjectV2ItemById(input: {projectId: $project, contentId: $pr}) {
            item {
              id
            }
          }
        }
    `

	fields := [][]string{
		{"-f", "project=" + p.ID},
		{"-f", "pr=" + nodeID},
		{"--jq", ".data.addProjectV2ItemById.item.id"},
	}

	return p.GraphQLQuery(q, fields)
}

func (p *Project) SetItemStatus(itemID, status string) error {
	// should this be a method of ProjectItem? (to do this we'll need to figure out how to get all the fields and values

	if p.ProjectDetails == nil {
		return errors.New("project details not loaded yet")
	}

	fields := []ProjectItemField{
		{Name: "number", FieldID: p.FieldIDs["Status"], Type: ItemValueTypeSingleSelect, Value: status},
	}

	return p.UpdateItem(itemID, fields)
}

// ProjectItemsResult is the result of the project items query; for now we hard code the project
// fields we want (dueDate and type).
// TODO in the future we can make this configurable / get all of them
type ProjectItemsResult struct {
	Data struct {
		Organization struct {
			ProjectV2 struct {
				ID    string `json:"id"`
				Items struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []struct {
						ID     string `json:"id"`
						Type   string `json:"type"`
						Status *struct {
							SingleSelectOptionID string `json:"singleSelectOptionId"`
						}
						RequestType *struct {
							Text string `json:"text"`
						} `json:"requestType"`
						DueDate *struct {
							Date string `json:"date"`
						} `json:"dueDate"`
						FieldValues struct {
							Nodes []struct {
								Typename             string  `json:"__typename"`
								Text                 string  `json:"text"`
								Number               float64 `json:"number"`
								Date                 string  `json:"date"`
								SingleSelectOptionID string  `json:"singleSelectOptionId"`
								Field                struct {
									Name string `json:"name"`
								} `json:"field"`
							} `json:"nodes"`
						} `json:"fieldValues"`
						Content struct {
							ID    string `json:"id"`
							Title string `json:"title"`
							URL   string `json:"url"`
						} `json:"content"`
					} `json:"nodes"`
				} `json:"items"`
			} `json:"projectV2"`
		} `json:"organization"`
	} `json:"data"`
}

type ProjectItem struct {
	ID          string
	Type        string
	Title       string
	URL         string
	RequestType string
	DueDate     string
	Status      string
	NodeID      string                           // actual pr/issue node id
	FieldValues map[string]ProjectItemFieldValue // current board values keyed by field name
}

// GetItems returns all items in the project.
// todo: allow configure the fields we want to get
func (p *Project) GetItems() ([]ProjectItem, error) {
	q := `query=
		query($org: String!, $number: Int!, $cursor: String) {
			organization(login: $org) {
				projectV2(number: $number) {
					id
					items(first: 100, after: $cursor) {
						pageInfo {
							hasNextPage
							endCursor
						}
						nodes {
							id
							type
							requestType:fieldValueByName(name:"Type") {
								... on ProjectV2ItemFieldTextValue {
									text
								}
      						}
      						dueDate:fieldValueByName(name:"Due Date") {
        						... on ProjectV2ItemFieldDateValue {
             						date
           						}
      						}
							status:fieldValueByName(name:"Status") {
								... on ProjectV2ItemFieldSingleSelectValue {
									singleSelectOptionId: optionId
								}
							}
							fieldValues(first: 50) {
								nodes {
									__typename
									... on ProjectV2ItemFieldTextValue {
										text
										field { ... on ProjectV2FieldCommon { name } }
									}
									... on ProjectV2ItemFieldNumberValue {
										number
										field { ... on ProjectV2FieldCommon { name } }
									}
									... on ProjectV2ItemFieldDateValue {
										date
										field { ... on ProjectV2FieldCommon { name } }
									}
									... on ProjectV2ItemFieldSingleSelectValue {
										singleSelectOptionId: optionId
										field { ... on ProjectV2FieldCommon { name } }
									}
								}
							}
							content {
								... on Issue {
									id
									title
									url
								}
								... on PullRequest {
									id
									title
									url
								}
							}
						}
					}
				}
			}
		}
    `

	var allItems []ProjectItem
	var cursor string

	for {
		params := [][]string{
			{"-f", "org=" + p.Owner},
			{"-F", "number=" + strconv.Itoa(p.Number)},
		}
		if cursor != "" {
			params = append(params, []string{"-f", "cursor=" + cursor})
		}

		var result ProjectItemsResult
		if err := p.GraphQLQueryUnmarshal(q, params, &result); err != nil {
			return nil, err
		}

		for _, i := range result.Data.Organization.ProjectV2.Items.Nodes {
			item := ProjectItem{
				ID:     i.ID,
				Type:   i.Type,
				Title:  i.Content.Title,
				URL:    i.Content.URL,
				NodeID: i.Content.ID,
			}

			if i.Status != nil {
				item.Status = i.Status.SingleSelectOptionID
			}
			if i.RequestType != nil {
				item.RequestType = i.RequestType.Text
			}
			if i.DueDate != nil {
				item.DueDate = i.DueDate.Date
			}

			item.FieldValues = map[string]ProjectItemFieldValue{}
			for _, fv := range i.FieldValues.Nodes {
				switch fv.Typename {
				case "ProjectV2ItemFieldTextValue":
					item.FieldValues[fv.Field.Name] = ProjectItemFieldValue{Type: ItemValueTypeText, Value: fv.Text}
				case "ProjectV2ItemFieldNumberValue":
					item.FieldValues[fv.Field.Name] = ProjectItemFieldValue{Type: ItemValueTypeNumber, Value: fv.Number}
				case "ProjectV2ItemFieldDateValue":
					item.FieldValues[fv.Field.Name] = ProjectItemFieldValue{Type: ItemValueTypeDate, Value: fv.Date}
				case "ProjectV2ItemFieldSingleSelectValue":
					item.FieldValues[fv.Field.Name] = ProjectItemFieldValue{Type: ItemValueTypeSingleSelect, Value: fv.SingleSelectOptionID}
				}
			}

			allItems = append(allItems, item)
		}

		if !result.Data.Organization.ProjectV2.Items.PageInfo.HasNextPage {
			break
		}
		cursor = result.Data.Organization.ProjectV2.Items.PageInfo.EndCursor
	}

	return allItems, nil
}

// ItemValueType is the type of a project item field value (text, number, single select, or date).
type ItemValueType int

const (
	ItemValueTypeText ItemValueType = iota
	ItemValueTypeNumber
	ItemValueTypeSingleSelect
	ItemValueTypeDate
)

func (t ItemValueType) String() string {
	switch t {
	case ItemValueTypeText:
		return "text"
	case ItemValueTypeNumber:
		return "number"
	case ItemValueTypeSingleSelect:
		return "select"
	case ItemValueTypeDate:
		return "date"
	default:
		return "unknown"
	}
}

// ProjectItemField represents a single field update for the project item.
// Type should be either "text" or "number".
type ProjectItemField struct {
	Name    string // A short name for this field (used in GraphQL alias, e.g. "set_key")
	FieldID string // The GraphQL ID of the field
	Type    ItemValueType
	Value   any
}

// UpdateItem updates the fields of a project item by building a dynamic GraphQL mutation.
func (p *Project) UpdateItem(itemID string, fields []ProjectItemField) error {
	// We'll build the mutation parts dynamically; Always include project and item as variables
	varDefs := []string{"$project:ID!", "$item:ID!"}
	setCalls := []string{}
	params := [][]string{
		{"-f", "project=" + p.ID},
		{"-f", "item=" + itemID},
	}

	// For each field, we create a pair of variables: one for the fieldId, and one for the value
	// We'll name them based on the field's index to ensure uniqueness.
	for _, f := range fields {
		fieldAlias := f.Name
		if fieldAlias == "" {
			return errors.New("field name cannot be empty")
		}
		fieldIDVar := fmt.Sprintf("$%s_field", fieldAlias)
		fieldValueVar := fmt.Sprintf("$%s_value", fieldAlias)

		// Validate the field id it can never be empty
		if f.FieldID == "" {
			return fmt.Errorf("field ID for %s is empty", fieldAlias)
		}

		// Variable definitions based on Type
		varDefs = append(varDefs, fieldIDVar+":ID!")
		switch f.Type {
		case ItemValueTypeText, ItemValueTypeSingleSelect:
			varDefs = append(varDefs, fieldValueVar+":String!")
		case ItemValueTypeNumber:
			varDefs = append(varDefs, fieldValueVar+":Float!")
		case ItemValueTypeDate:
			varDefs = append(varDefs, fieldValueVar+":Date!")
		default:
			return fmt.Errorf("unsupported value type: %v for %s ", f.Type, fieldAlias)
		}

		// Add parameters for this field
		params = append(params, []string{"-f", fmt.Sprintf("%s_field=%s", fieldAlias, f.FieldID)})
		switch f.Type {
		case ItemValueTypeText, ItemValueTypeDate, ItemValueTypeSingleSelect:
			params = append(params, []string{"-f", fmt.Sprintf("%s_value=%v", fieldAlias, f.Value)})
		case ItemValueTypeNumber:
			// Use -F so the value is recognized as a JSON number
			params = append(params, []string{"-F", fmt.Sprintf("%s_value=%v", fieldAlias, f.Value)})
		}

		// Build the update call for the mutation
		var valuePart string
		switch f.Type {
		case ItemValueTypeText:
			valuePart = fmt.Sprintf("value: { text: %s }", fieldValueVar)
		case ItemValueTypeNumber:
			valuePart = fmt.Sprintf("value: { number: %s }", fieldValueVar)
		case ItemValueTypeDate:
			valuePart = fmt.Sprintf("value: { date: %s }", fieldValueVar)
		case ItemValueTypeSingleSelect:
			valuePart = fmt.Sprintf("value: { singleSelectOptionId: %s }", fieldValueVar)
		}

		setCalls = append(setCalls, fmt.Sprintf(`
  %s: updateProjectV2ItemFieldValue(input: {
    projectId: $project
    itemId: $item
    fieldId: %s
    %s
  }) {
    projectV2Item { id }
  }`, "set_"+fieldAlias, fieldIDVar, valuePart))
	}

	// Now assemble the full mutation
	mutation := fmt.Sprintf(`query=mutation(
  %s
) {
  %s
}`, strings.Join(varDefs, ", "), strings.Join(setCalls, "\n"))

	out, err := p.GraphQLQuery(mutation, params)
	if err != nil {
		outStr := ""
		if out != nil {
			outStr = *out
		}
		return fmt.Errorf("error updating project item: %w\noutput: %s\nmutation: %s", err, outStr, mutation)
	}

	return nil
}

// ProjectItemFieldValue holds a field value read from a project item.
type ProjectItemFieldValue struct {
	Type  ItemValueType
	Value any // string for text/date/singleSelect option ID, float64 for number
}

// GetItemFieldValuesByNodeID looks up the project item for a given content node ID (e.g. an issue)
// and returns the field values for the requested field names. The returned map is keyed by field name.
// If the item is not found in the project, returns nil map with no error.
func (p *Project) GetItemFieldValuesByNodeID(contentNodeID string, fieldNames []string) (map[string]ProjectItemFieldValue, error) {
	if p.ProjectDetails == nil {
		return nil, errors.New("project details not loaded yet")
	}

	// Build dynamic fieldValueByName queries for each requested field
	var fieldFragments strings.Builder
	for i, name := range fieldNames {
		alias := fmt.Sprintf("f%d", i)
		// Escape quotes and backslashes to prevent GraphQL injection
		safeName := strings.ReplaceAll(name, `\`, `\\`)
		safeName = strings.ReplaceAll(safeName, `"`, `\"`)
		fmt.Fprintf(&fieldFragments, `
						%s:fieldValueByName(name:"%s") {
							... on ProjectV2ItemFieldTextValue {
								__typename
								text
							}
							... on ProjectV2ItemFieldNumberValue {
								__typename
								number
							}
							... on ProjectV2ItemFieldDateValue {
								__typename
								date
							}
							... on ProjectV2ItemFieldSingleSelectValue {
								__typename
								singleSelectOptionId: optionId
							}
						}`, alias, safeName)
	}

	q := fmt.Sprintf(`query=
		query($org: String!, $number: Int!, $cursor: String) {
			organization(login: $org) {
				projectV2(number: $number) {
					items(first: 100, after: $cursor) {
						pageInfo {
							hasNextPage
							endCursor
						}
						nodes {
							id
							content {
								... on Issue {
									id
								}
								... on PullRequest {
									id
								}
							}
							%s
						}
					}
				}
			}
		}
	`, fieldFragments.String())

	params := [][]string{
		{"-f", "org=" + p.Owner},
		{"-F", "number=" + strconv.Itoa(p.Number)},
	}

	// We need a dynamic result type since field names are dynamic
	type fieldValue struct {
		Typename             string  `json:"__typename"`
		Text                 string  `json:"text"`
		Number               float64 `json:"number"`
		Date                 string  `json:"date"`
		SingleSelectOptionID string  `json:"singleSelectOptionId"`
	}

	type itemNode struct {
		ID      string `json:"id"`
		Content struct {
			ID string `json:"id"`
		} `json:"content"`
		// Dynamic field values will be parsed from raw JSON
	}

	type queryResult struct {
		Data struct {
			Organization struct {
				ProjectV2 struct {
					Items struct {
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []json.RawMessage `json:"nodes"`
					} `json:"items"`
				} `json:"projectV2"`
			} `json:"organization"`
		} `json:"data"`
	}

	var cursor string
	for {
		queryParams := make([][]string, 0, len(params)+1)
		queryParams = append(queryParams, params...)
		if cursor != "" {
			queryParams = append(queryParams, []string{"-f", "cursor=" + cursor})
		}

		var result queryResult
		if err := p.GraphQLQueryUnmarshal(q, queryParams, &result); err != nil {
			return nil, fmt.Errorf("querying project items for field values: %w", err)
		}

		for _, rawNode := range result.Data.Organization.ProjectV2.Items.Nodes {
			// Parse the node to get the content ID
			var node itemNode
			if err := json.Unmarshal(rawNode, &node); err != nil {
				continue
			}

			if node.Content.ID != contentNodeID {
				continue
			}

			// Found the item — parse the field values
			var rawMap map[string]json.RawMessage
			if err := json.Unmarshal(rawNode, &rawMap); err != nil {
				return nil, fmt.Errorf("parsing item fields: %w", err)
			}

			result := map[string]ProjectItemFieldValue{}
			for i, name := range fieldNames {
				alias := fmt.Sprintf("f%d", i)
				raw, ok := rawMap[alias]
				if !ok || string(raw) == "null" || string(raw) == "{}" {
					continue
				}

				var fv fieldValue
				if err := json.Unmarshal(raw, &fv); err != nil {
					continue
				}

				switch fv.Typename {
				case "ProjectV2ItemFieldTextValue":
					result[name] = ProjectItemFieldValue{Type: ItemValueTypeText, Value: fv.Text}
				case "ProjectV2ItemFieldNumberValue":
					result[name] = ProjectItemFieldValue{Type: ItemValueTypeNumber, Value: fv.Number}
				case "ProjectV2ItemFieldDateValue":
					result[name] = ProjectItemFieldValue{Type: ItemValueTypeDate, Value: fv.Date}
				case "ProjectV2ItemFieldSingleSelectValue":
					result[name] = ProjectItemFieldValue{Type: ItemValueTypeSingleSelect, Value: fv.SingleSelectOptionID}
				}
			}

			return result, nil
		}

		if !result.Data.Organization.ProjectV2.Items.PageInfo.HasNextPage {
			break
		}
		cursor = result.Data.Organization.ProjectV2.Items.PageInfo.EndCursor
	}

	// Item not found in project
	return nil, nil
}
