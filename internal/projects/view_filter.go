package projects

import (
	"fmt"
	"strings"
)

// ConvertFieldFiltersToQueryString converts FieldFilter array to GitHub's query string format
//
// Converts internal FieldFilter representation to GitHub Projects query syntax.
//
// Examples:
//
//	Input: [{FieldName:"Status", Values:["Blocked"]}]
//	Output: "Status:Blocked"
//
//	Input: [{FieldName:"Status", Values:["Blocked", "In Progress"]}]
//	Output: "Status:Blocked,In Progress"
//	(Note: "In Progress" is quoted because it contains a space)
//
//	Input: [{FieldName:"Status", Values:["Blocked"]}, {FieldName:"Priority", Values:["High", "Critical"]}]
//	Output: "Status:Blocked Priority:High,Critical"
//
// Query syntax rules:
// - Multiple values for same field: comma-separated (OR logic)
// - Multiple fields: space-separated (AND logic)
// - Values with spaces: wrapped in quotes
func ConvertFieldFiltersToQueryString(filters []FieldFilter) string {
	if len(filters) == 0 {
		return ""
	}

	var parts []string
	for _, filter := range filters {
		// Escape values with spaces using quotes
		var escapedValues []string
		for _, value := range filter.Values {
			if strings.Contains(value, " ") {
				// Wrap in quotes and escape any existing quotes
				escaped := strings.ReplaceAll(value, `"`, `\"`)
				escapedValues = append(escapedValues, fmt.Sprintf(`"%s"`, escaped))
			} else {
				escapedValues = append(escapedValues, value)
			}
		}

		// Join multiple values with comma (OR logic within field)
		valueStr := strings.Join(escapedValues, ",")

		// Create field:values pair
		parts = append(parts, fmt.Sprintf("%s:%s", filter.FieldName, valueStr))
	}

	// Join multiple fields with space (AND logic between fields)
	return strings.Join(parts, " ")
}

// parseQueryStringFilter parses GitHub's query string filter format
//
// Format: "field1:value1,value2 field2:value3"
// Example: "type:Epic,Initiative quarter:FY26Q2"
//
// Rules:
// - Space-separated field:value pairs
// - Comma-separated values within a field
// - Field names are capitalized as they appear
func parseQueryStringFilter(filterString string) ([]FieldFilter, error) {
	filterString = strings.TrimSpace(filterString)
	if filterString == "" {
		return []FieldFilter{}, nil
	}

	var filters []FieldFilter

	// Split by spaces to get field:value pairs
	pairs := strings.Fields(filterString)

	for _, pair := range pairs {
		// Split by first colon to separate field name and values
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			// Skip malformed pairs
			continue
		}

		fieldName := strings.TrimSpace(parts[0])
		valuesStr := strings.TrimSpace(parts[1])

		if fieldName == "" || valuesStr == "" {
			continue
		}

		// Split values by comma
		values := strings.Split(valuesStr, ",")

		// Trim whitespace from each value
		for i, v := range values {
			values[i] = strings.TrimSpace(v)
		}

		filters = append(filters, FieldFilter{
			FieldName: fieldName,
			Values:    values,
		})
	}

	return filters, nil
}

// MergeFilters combines view-based filters with additional user filters
//
// Strategy: View filters act as the base, user filters are added on top
// - If user specifies a filter for the same field as the view, user filter takes precedence
// - Otherwise, both filters are included (AND logic between different fields)
//
// Example:
//
//	View filters: Status=Blocked
//	User filters: Priority=High
//	Result: Status=Blocked AND Priority=High
//
// Example with override:
//
//	View filters: Status=Blocked
//	User filters: Status=InProgress
//	Result: Status=InProgress (user overrides view)
func MergeFilters(viewFilters, userFilters []FieldFilter) []FieldFilter {
	// If no view filters, return user filters as-is
	if len(viewFilters) == 0 {
		return userFilters
	}

	// If no user filters, return view filters as-is
	if len(userFilters) == 0 {
		return viewFilters
	}

	// Build a map of user filters by field name for quick lookup
	userFilterMap := make(map[string]FieldFilter)
	for _, filter := range userFilters {
		userFilterMap[filter.FieldName] = filter
	}

	// Start with view filters, but replace with user filter if same field
	var merged []FieldFilter
	addedFields := make(map[string]bool)

	for _, viewFilter := range viewFilters {
		if userFilter, exists := userFilterMap[viewFilter.FieldName]; exists {
			// User specified filter for same field - use user's version
			merged = append(merged, userFilter)
			addedFields[viewFilter.FieldName] = true
		} else {
			// No user override - use view filter
			merged = append(merged, viewFilter)
			addedFields[viewFilter.FieldName] = true
		}
	}

	// Add any user filters that weren't in view filters
	for _, userFilter := range userFilters {
		if !addedFields[userFilter.FieldName] {
			merged = append(merged, userFilter)
		}
	}

	return merged
}

// FormatFilterSummary creates a human-readable summary of filters
// Useful for logging and error messages
func FormatFilterSummary(filters []FieldFilter) string {
	if len(filters) == 0 {
		return "no filters"
	}

	var parts []string
	for _, filter := range filters {
		values := strings.Join(filter.Values, ", ")
		parts = append(parts, fmt.Sprintf("%s=[%s]", filter.FieldName, values))
	}

	return strings.Join(parts, " AND ")
}
