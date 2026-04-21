package format

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// GroupMode specifies how rows are partitioned into groups.
type GroupMode int

const (
	GroupByAssignee GroupMode = iota
	GroupByLabel
	GroupByField
)

// GroupConfig holds the grouping mode and optional pattern (glob or field name).
type GroupConfig struct {
	Mode    GroupMode
	Pattern string
}

// RowGroup is a titled collection of rows.
type RowGroup struct {
	Title string
	Rows  []Row
}

// ParseGroupBy parses a raw grouping spec into a GroupConfig.
//
// Valid formats:
//
//	"assignee"       → GroupByAssignee
//	"label:<glob>"   → GroupByLabel with pattern
//	"field:<name>"   → GroupByField with pattern
func ParseGroupBy(raw string) (GroupConfig, error) {
	if raw == "" {
		return GroupConfig{}, fmt.Errorf("grouping spec must not be empty")
	}

	switch {
	case raw == "assignee":
		return GroupConfig{Mode: GroupByAssignee}, nil

	case strings.HasPrefix(raw, "label:"):
		pattern := strings.TrimPrefix(raw, "label:")
		if pattern == "" {
			return GroupConfig{}, fmt.Errorf("label grouping requires a glob pattern, e.g. label:team-*")
		}
		// Validate the glob pattern.
		if _, err := filepath.Match(pattern, ""); err != nil {
			return GroupConfig{}, fmt.Errorf("invalid label glob pattern %q: %w", pattern, err)
		}
		return GroupConfig{Mode: GroupByLabel, Pattern: pattern}, nil

	case strings.HasPrefix(raw, "field:"):
		name := strings.TrimPrefix(raw, "field:")
		if name == "" {
			return GroupConfig{}, fmt.Errorf("field grouping requires a field name, e.g. field:Priority")
		}
		return GroupConfig{Mode: GroupByField, Pattern: name}, nil

	default:
		return GroupConfig{}, fmt.Errorf("unknown grouping spec %q; expected assignee, label:<glob>, or field:<name>", raw)
	}
}

// GroupRows partitions rows into RowGroups according to config.
// Each group's rows are sorted by target date. Groups are sorted alphabetically,
// with the fallback group ("Unassigned" / "Other") placed last.
func GroupRows(rows []Row, config GroupConfig) []RowGroup {
	if len(rows) == 0 {
		return nil
	}

	grouped := make(map[string][]Row)

	for _, row := range rows {
		key := groupKey(row, config)
		grouped[key] = append(grouped[key], row)
	}

	fallback := fallbackTitle(config)
	var keys []string
	for k := range grouped {
		if k != fallback {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if _, hasFallback := grouped[fallback]; hasFallback {
		keys = append(keys, fallback)
	}

	result := make([]RowGroup, 0, len(keys))
	for _, k := range keys {
		r := grouped[k]
		SortRowsByTargetDate(r)
		result = append(result, RowGroup{Title: k, Rows: r})
	}
	return result
}

const (
	fallbackAssignee = "Unassigned"
	fallbackOther    = "Other"
)

// groupKey returns the group key for a single row.
func groupKey(row Row, config GroupConfig) string {
	switch config.Mode {
	case GroupByAssignee:
		if len(row.Assignees) > 0 {
			return row.Assignees[0]
		}
		return fallbackAssignee

	case GroupByLabel:
		for _, label := range row.Labels {
			matched, err := filepath.Match(config.Pattern, label)
			if err == nil && matched {
				return label
			}
		}
		return fallbackOther

	case GroupByField:
		if row.ExtraColumns != nil {
			if val, ok := row.ExtraColumns[config.Pattern]; ok && val != "" {
				return val
			}
		}
		return fallbackOther
	}
	return fallbackOther
}

// fallbackTitle returns the fallback group name for the given mode.
func fallbackTitle(config GroupConfig) string {
	if config.Mode == GroupByAssignee {
		return fallbackAssignee
	}
	return fallbackOther
}
