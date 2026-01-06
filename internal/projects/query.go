package projects

import "fmt"

// GraphQL query template for fetching project items
// The %s placeholder will be replaced with either "organization" or "user"
// The query parameter allows server-side filtering using GitHub's filter syntax
const projectItemsQueryTemplate = `
query($owner: String!, $number: Int!, $first: Int!, $cursor: String, $query: String) {
  %s(login: $owner) {
    projectV2(number: $number) {
      id
      title
      items(first: $first, after: $cursor, query: $query) {
        nodes {
          id
          type
          content {
            ... on Issue {
              id
              number
              url
              repository {
                owner {
                  login
                }
                name
              }
            }
            ... on PullRequest {
              id
              number
              url
              repository {
                owner {
                  login
                }
                name
              }
            }
            ... on DraftIssue {
              id
              title
            }
          }
        }
        pageInfo {
          hasNextPage
          endCursor
        }
      }
    }
  }
}
`

// buildProjectQuery builds a GraphQL query string for the given project type
func buildProjectQuery(projectType ProjectType) string {
	var ownerType string
	switch projectType {
	case ProjectTypeOrg:
		ownerType = "organization"
	case ProjectTypeUser:
		ownerType = "user"
	default:
		ownerType = "organization"
	}
	return fmt.Sprintf(projectItemsQueryTemplate, ownerType)
}

// GraphQL query template for fetching project views
// The %s placeholder will be replaced with either "organization" or "user"
const projectViewsQueryTemplate = `
query($owner: String!, $number: Int!) {
  %s(login: $owner) {
    projectV2(number: $number) {
      id
      title
      views(first: 20) {
        nodes {
          id
          name
          filter
          layout
        }
      }
    }
  }
}
`

// buildProjectViewsQuery builds a GraphQL query string for fetching views
func buildProjectViewsQuery(projectType ProjectType) string {
	var ownerType string
	switch projectType {
	case ProjectTypeOrg:
		ownerType = "organization"
	case ProjectTypeUser:
		ownerType = "user"
	default:
		ownerType = "organization"
	}
	return fmt.Sprintf(projectViewsQueryTemplate, ownerType)
}

// graphQLRequest represents a GraphQL request payload
type graphQLRequest struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

// graphQLResponse represents a GraphQL response
type graphQLResponse struct {
	Data   *projectData   `json:"data,omitempty"`
	Errors []graphQLError `json:"errors,omitempty"`
}

// graphQLError represents a GraphQL error
type graphQLError struct {
	Message string   `json:"message"`
	Type    string   `json:"type,omitempty"`
	Path    []string `json:"path,omitempty"`
}

// projectData represents the data field in GraphQL response
type projectData struct {
	Organization *projectV2Wrapper `json:"organization,omitempty"`
	User         *projectV2Wrapper `json:"user,omitempty"`
}

// GetProject returns the project data based on the project type
func (pd *projectData) GetProject() *projectV2 {
	if pd.Organization != nil {
		return pd.Organization.ProjectV2
	}
	if pd.User != nil {
		return pd.User.ProjectV2
	}
	return nil
}

// projectV2Wrapper wraps the projectV2 field
type projectV2Wrapper struct {
	ProjectV2 *projectV2 `json:"projectV2"`
}

// projectV2 represents a GitHub Project V2
type projectV2 struct {
	ID     string        `json:"id"`
	Title  string        `json:"title"`
	Fields projectFields `json:"fields"`
	Items  projectItems  `json:"items,omitempty"` // Only present in items query
	Views  projectViews  `json:"views,omitempty"` // Only present in views query
}

// projectFields represents the fields collection
type projectFields struct {
	Nodes []projectField `json:"nodes"`
}

// projectField represents a project field definition
type projectField struct {
	ID      string               `json:"id"`
	Name    string               `json:"name"`
	Options []projectFieldOption `json:"options,omitempty"` // For single-select fields
}

// projectFieldOption represents an option in a single-select field
type projectFieldOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// projectItems represents the items collection with pagination
type projectItems struct {
	Nodes    []projectItemNode `json:"nodes"`
	PageInfo pageInfo          `json:"pageInfo"`
}

// pageInfo represents pagination information
type pageInfo struct {
	HasNextPage bool    `json:"hasNextPage"`
	EndCursor   *string `json:"endCursor"`
}

// projectViews represents the views collection
type projectViews struct {
	Nodes []projectViewNode `json:"nodes"`
}

// projectViewNode represents a single project view from GraphQL response
type projectViewNode struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Filter *string `json:"filter,omitempty"` // May be null if view has no filter
	Layout string  `json:"layout"`
}

// projectItemNode represents a single project item
type projectItemNode struct {
	ID          string              `json:"id"`
	Type        string              `json:"type"` // ISSUE, PULL_REQUEST, DRAFT_ISSUE
	Content     *projectItemContent `json:"content,omitempty"`
	FieldValues projectFieldValues  `json:"fieldValues"`
}

// projectItemContent represents the content of a project item (issue/PR/draft)
type projectItemContent struct {
	// Common fields
	ID    string `json:"id"`
	Title string `json:"title,omitempty"` // For draft issues

	// Issue/PR specific fields
	Number     *int               `json:"number,omitempty"`
	URL        string             `json:"url,omitempty"`
	Repository *contentRepository `json:"repository,omitempty"`
}

// contentRepository represents the repository info in item content
type contentRepository struct {
	Owner repositoryOwner `json:"owner"`
	Name  string          `json:"name"`
}

// repositoryOwner represents the owner of a repository
type repositoryOwner struct {
	Login string `json:"login"`
}

// projectFieldValues represents the field values collection
type projectFieldValues struct {
	Nodes []projectFieldValueNode `json:"nodes"`
}

// projectFieldValueNode represents a single field value
// Uses inline fragments for different field types
type projectFieldValueNode struct {
	// Common to all types
	Field *projectFieldRef `json:"field,omitempty"`

	// Type-specific values
	Text   *string  `json:"text,omitempty"`   // For text fields
	Name   *string  `json:"name,omitempty"`   // For single-select fields
	Date   *string  `json:"date,omitempty"`   // For date fields (ISO 8601)
	Number *float64 `json:"number,omitempty"` // For number fields
}

// projectFieldRef represents a reference to a field definition
type projectFieldRef struct {
	Name string `json:"name"`
}
