package fddp

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrLimitExceeded = errors.New("fddp: limit exceeded")

var ErrQueryLimitExceeded = ErrLimitExceeded

type QueryLimits struct {
	MaxFields          int
	MaxResources       int
	MaxCollectionFirst int
	MaxSelectionFields int
	MaxExpandDepth     int
	MaxExpandRelations int
	MaxFilterFields    int
	MaxOrderBy         int
	MaxCost            int
	MaxBodyBytes       int
	MaxQueryDepth      int
	MaxQueryNodes      int
	Timeout            time.Duration

	FieldCost          int
	ResourceCost       int
	SelectionFieldCost int
	ExpandCost         int
	FilterCost         int
	OrderByCost        int
	PageItemCost       int
	TotalCountCost     int
	ContainsCost       int
}

type QueryCost struct {
	Fields          int `json:"fields"`
	Resources       int `json:"resources"`
	SelectionFields int `json:"selectionFields"`
	ExpandRelations int `json:"expandRelations"`
	FilterFields    int `json:"filterFields"`
	OrderBy         int `json:"orderBy"`
	CollectionFirst int `json:"collectionFirst"`
	TotalCost       int `json:"totalCost"`
}

func DefaultQueryLimits() QueryLimits {
	return QueryLimits{
		MaxFields:          50,
		MaxResources:       5,
		MaxCollectionFirst: 100,
		MaxSelectionFields: 50,
		MaxExpandDepth:     1,
		MaxExpandRelations: 5,
		MaxFilterFields:    10,
		MaxOrderBy:         3,
		MaxCost:            250,
		MaxBodyBytes:       1 << 20,
		MaxQueryDepth:      20,
		MaxQueryNodes:      500,
		Timeout:            2 * time.Second,

		FieldCost:          1,
		ResourceCost:       10,
		SelectionFieldCost: 1,
		ExpandCost:         15,
		FilterCost:         5,
		OrderByCost:        3,
		PageItemCost:       1,
		TotalCountCost:     30,
		ContainsCost:       20,
	}
}

func NoQueryLimits() QueryLimits {
	return QueryLimits{
		MaxFields:          -1,
		MaxResources:       -1,
		MaxCollectionFirst: -1,
		MaxSelectionFields: -1,
		MaxExpandDepth:     -1,
		MaxExpandRelations: -1,
		MaxFilterFields:    -1,
		MaxOrderBy:         -1,
		MaxCost:            -1,
		MaxBodyBytes:       -1,
		MaxQueryDepth:      -1,
		MaxQueryNodes:      -1,
		Timeout:            -1,
	}
}

func (limits QueryLimits) withDefaults() QueryLimits {
	defaults := DefaultQueryLimits()
	if limits.MaxFields == 0 {
		limits.MaxFields = defaults.MaxFields
	}
	if limits.MaxResources == 0 {
		limits.MaxResources = defaults.MaxResources
	}
	if limits.MaxCollectionFirst == 0 {
		limits.MaxCollectionFirst = defaults.MaxCollectionFirst
	}
	if limits.MaxSelectionFields == 0 {
		limits.MaxSelectionFields = defaults.MaxSelectionFields
	}
	if limits.MaxExpandDepth == 0 {
		limits.MaxExpandDepth = defaults.MaxExpandDepth
	}
	if limits.MaxExpandRelations == 0 {
		limits.MaxExpandRelations = defaults.MaxExpandRelations
	}
	if limits.MaxFilterFields == 0 {
		limits.MaxFilterFields = defaults.MaxFilterFields
	}
	if limits.MaxOrderBy == 0 {
		limits.MaxOrderBy = defaults.MaxOrderBy
	}
	if limits.MaxCost == 0 {
		limits.MaxCost = defaults.MaxCost
	}
	if limits.MaxBodyBytes == 0 {
		limits.MaxBodyBytes = defaults.MaxBodyBytes
	}
	if limits.MaxQueryDepth == 0 {
		limits.MaxQueryDepth = defaults.MaxQueryDepth
	}
	if limits.MaxQueryNodes == 0 {
		limits.MaxQueryNodes = defaults.MaxQueryNodes
	}
	if limits.Timeout == 0 {
		limits.Timeout = defaults.Timeout
	}
	if limits.FieldCost == 0 {
		limits.FieldCost = defaults.FieldCost
	}
	if limits.ResourceCost == 0 {
		limits.ResourceCost = defaults.ResourceCost
	}
	if limits.SelectionFieldCost == 0 {
		limits.SelectionFieldCost = defaults.SelectionFieldCost
	}
	if limits.ExpandCost == 0 {
		limits.ExpandCost = defaults.ExpandCost
	}
	if limits.FilterCost == 0 {
		limits.FilterCost = defaults.FilterCost
	}
	if limits.OrderByCost == 0 {
		limits.OrderByCost = defaults.OrderByCost
	}
	if limits.PageItemCost == 0 {
		limits.PageItemCost = defaults.PageItemCost
	}
	if limits.TotalCountCost == 0 {
		limits.TotalCountCost = defaults.TotalCountCost
	}
	if limits.ContainsCost == 0 {
		limits.ContainsCost = defaults.ContainsCost
	}
	return limits
}

func (limits QueryLimits) disabled() bool {
	return limits.MaxFields < 0 &&
		limits.MaxResources < 0 &&
		limits.MaxCollectionFirst < 0 &&
		limits.MaxSelectionFields < 0 &&
		limits.MaxExpandDepth < 0 &&
		limits.MaxExpandRelations < 0 &&
		limits.MaxFilterFields < 0 &&
		limits.MaxOrderBy < 0 &&
		limits.MaxCost < 0 &&
		limits.MaxBodyBytes < 0 &&
		limits.MaxQueryDepth < 0 &&
		limits.MaxQueryNodes < 0
}

func validateQueryPlanLimits(plan QueryPlan, limits QueryLimits) (QueryCost, error) {
	if limits.disabled() {
		return estimateQueryCost(plan, QueryLimits{}), nil
	}
	limits = limits.withDefaults()
	cost := estimateQueryCost(plan, limits)

	if limitExceeded(len(plan.Fields), limits.MaxFields) {
		return cost, fmt.Errorf("field count %d exceeds limit %d", len(plan.Fields), limits.MaxFields)
	}
	if limitExceeded(len(plan.Resources), limits.MaxResources) {
		return cost, fmt.Errorf("resource count %d exceeds limit %d", len(plan.Resources), limits.MaxResources)
	}
	for _, resource := range plan.Resources {
		selectionFields := countSelectionFields(resource.Selection)
		if limitExceeded(selectionFields, limits.MaxSelectionFields) {
			return cost, fmt.Errorf("selection field count %d exceeds limit %d for %s", selectionFields, limits.MaxSelectionFields, resource.Path)
		}
		expandDepth := selectionExpandDepth(resource.Selection)
		if limitExceeded(expandDepth, limits.MaxExpandDepth) {
			return cost, fmt.Errorf("expand depth %d exceeds limit %d for %s", expandDepth, limits.MaxExpandDepth, resource.Path)
		}
		expandRelations := countExpandRelations(resource.Selection)
		if limitExceeded(expandRelations, limits.MaxExpandRelations) {
			return cost, fmt.Errorf("expand relation count %d exceeds limit %d for %s", expandRelations, limits.MaxExpandRelations, resource.Path)
		}
		if resource.Type == ResourceQueryCollection {
			first := resource.Collection.First
			if limitExceeded(first, limits.MaxCollectionFirst) {
				return cost, fmt.Errorf("collection first %d exceeds limit %d for %s", first, limits.MaxCollectionFirst, resource.Path)
			}
			if limitExceeded(len(resource.Collection.Filter), limits.MaxFilterFields) {
				return cost, fmt.Errorf("filter field count %d exceeds limit %d for %s", len(resource.Collection.Filter), limits.MaxFilterFields, resource.Path)
			}
			if limitExceeded(len(resource.Collection.OrderBy), limits.MaxOrderBy) {
				return cost, fmt.Errorf("orderBy count %d exceeds limit %d for %s", len(resource.Collection.OrderBy), limits.MaxOrderBy, resource.Path)
			}
		}
	}
	if limitExceeded(cost.TotalCost, limits.MaxCost) {
		return cost, fmt.Errorf("query cost %d exceeds limit %d", cost.TotalCost, limits.MaxCost)
	}
	return cost, nil
}

func limitExceeded(value int, limit int) bool {
	return limit >= 0 && value > limit
}

func estimateQueryCost(plan QueryPlan, limits QueryLimits) QueryCost {
	cost := QueryCost{
		Fields:    len(plan.Fields),
		Resources: len(plan.Resources),
	}
	cost.TotalCost += len(plan.Fields) * weight(limits.FieldCost, 1)
	cost.TotalCost += len(plan.Resources) * weight(limits.ResourceCost, 10)

	for _, resource := range plan.Resources {
		selectionFields := countSelectionFields(resource.Selection)
		expandRelations := countExpandRelations(resource.Selection)
		cost.SelectionFields += selectionFields
		cost.ExpandRelations += expandRelations
		cost.TotalCost += selectionFields * weight(limits.SelectionFieldCost, 1)
		cost.TotalCost += expandRelations * weight(limits.ExpandCost, 15)

		if resource.Type == ResourceQueryCollection {
			first := resource.Collection.First
			if first <= 0 {
				first = 1
			}
			cost.CollectionFirst += first
			cost.FilterFields += len(resource.Collection.Filter)
			cost.OrderBy += len(resource.Collection.OrderBy)
			cost.TotalCost += first * weight(limits.PageItemCost, 1)
			cost.TotalCost += len(resource.Collection.Filter) * weight(limits.FilterCost, 5)
			cost.TotalCost += len(resource.Collection.OrderBy) * weight(limits.OrderByCost, 3)
			cost.TotalCost += expensiveFilterCost(resource.Collection.Filter, limits)
		}
	}
	return cost
}

func weight(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func countSelectionFields(selection Selection) int {
	count := len(selection.Fields)
	for _, child := range selection.Expand {
		count += countSelectionFields(child)
	}
	return count
}

func countExpandRelations(selection Selection) int {
	count := len(selection.Expand)
	for _, child := range selection.Expand {
		count += countExpandRelations(child)
	}
	return count
}

func selectionExpandDepth(selection Selection) int {
	maxDepth := 0
	for _, child := range selection.Expand {
		depth := 1 + selectionExpandDepth(child)
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	return maxDepth
}

func expensiveFilterCost(filters map[string]any, limits QueryLimits) int {
	total := 0
	for _, value := range filters {
		total += expensiveValueCost(value, limits)
	}
	return total
}

func expensiveValueCost(value any, limits QueryLimits) int {
	operators, ok := value.(map[string]any)
	if !ok {
		return 0
	}
	total := 0
	for op, child := range operators {
		switch strings.ToLower(op) {
		case "like", "contains":
			total += weight(limits.ContainsCost, 20)
		}
		total += expensiveValueCost(child, limits)
	}
	return total
}
