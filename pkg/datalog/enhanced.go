package datalog

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type AggregationType string

const (
	AggregationCount AggregationType = "count"
	AggregationSum   AggregationType = "sum"
	AggregationMin   AggregationType = "min"
	AggregationMax   AggregationType = "max"
)

type SortDirection string

const (
	SortAsc  SortDirection = "asc"
	SortDesc SortDirection = "desc"
)

type QueryModifier struct {
	Aggregation *AggregationSpec
	GroupBy     []string
	SortBy      []SortSpec
	Limit       *int
	Offset      *int
}

type AggregationSpec struct {
	Type     AggregationType
	Variable string
	Alias    string
}

type SortSpec struct {
	Variable  string
	Direction SortDirection
}

type ParsedQuery struct {
	Atoms    []Atom
	Modifier *QueryModifier
}

func ParseEnhanced(query string) (*ParsedQuery, error) {
	modifier, baseQuery := extractModifiers(query)

	atoms, err := Parse(baseQuery)
	if err != nil {
		return nil, err
	}

	return &ParsedQuery{
		Atoms:    atoms,
		Modifier: modifier,
	}, nil
}

func extractModifiers(query string) (*QueryModifier, string) {
	modifier := &QueryModifier{}

	upperQuery := strings.ToUpper(query)

	if idx := strings.Index(upperQuery, "LIMIT"); idx != -1 {
		rest := strings.TrimSpace(query[idx+5:])
		parts := strings.Fields(rest)
		if len(parts) > 0 {
			if limit, err := strconv.Atoi(parts[0]); err == nil {
				modifier.Limit = &limit
			}
		}
	}

	if idx := strings.Index(upperQuery, "OFFSET"); idx != -1 {
		rest := strings.TrimSpace(query[idx+6:])
		parts := strings.Fields(rest)
		if len(parts) > 0 {
			if offset, err := strconv.Atoi(parts[0]); err == nil {
				modifier.Offset = &offset
			}
		}
	}

	re := regexp.MustCompile(`(?i)\bORDER\s+BY\s+([^\sLIMIT]+.*?)(?:\s+GROUP|\s+COUNT|\s+SUM|\s+MIN|\s+MAX|\s*$)`)
	orderByMatch := re.FindStringSubmatch(query)
	if orderByMatch == nil {
		re = regexp.MustCompile(`(?i)\bORDER\s+BY\s+([^\s,]+)`)
		orderByMatch = re.FindStringSubmatch(query)
	}
	if orderByMatch != nil {
		orderPart := strings.TrimSpace(orderByMatch[1])
		orderPart = strings.TrimSuffix(orderPart, ")")
		orderVars := strings.Split(orderPart, ",")
		for _, v := range orderVars {
			v = strings.TrimSpace(v)
			upperVar := strings.ToUpper(v)
			var dir SortDirection
			if strings.HasSuffix(upperVar, "DESC") {
				dir = SortDesc
				v = strings.TrimSuffix(v, "DESC")
				v = strings.TrimSpace(v)
			} else if strings.HasSuffix(upperVar, "ASC") {
				dir = SortAsc
				v = strings.TrimSuffix(v, "ASC")
				v = strings.TrimSpace(v)
			} else {
				dir = SortAsc
			}
			v = strings.TrimPrefix(v, "?")
			modifier.SortBy = append(modifier.SortBy, SortSpec{
				Variable:  "?" + v,
				Direction: dir,
			})
		}
	}

	re = regexp.MustCompile(`(?i)\bGROUP\s+BY\s+([^\sORDER]+.*?)(?:\s+ORDER|\s+COUNT|\s+SUM|\s+MIN|\s+MAX|\s+LIMIT|\s*$)`)
	groupByMatch := re.FindStringSubmatch(query)
	if groupByMatch == nil {
		re = regexp.MustCompile(`(?i)\bGROUP\s+BY\s+([^\s,]+)`)
		groupByMatch = re.FindStringSubmatch(query)
	}
	if groupByMatch != nil {
		groupPart := strings.TrimSpace(groupByMatch[1])
		groupPart = strings.TrimSuffix(groupPart, ")")
		groupVars := strings.Split(groupPart, ",")
		for _, v := range groupVars {
			v = strings.TrimSpace(v)
			v = strings.TrimPrefix(v, "?")
			modifier.GroupBy = append(modifier.GroupBy, "?"+v)
		}
	}

	aggCountMatch := regexp.MustCompile(`(?i)COUNT\s*\(\s*([*\w]+)\s*\)\s*AS\s+(\w+)`).FindStringSubmatch(query)
	if aggCountMatch != nil {
		modifier.Aggregation = &AggregationSpec{
			Type:     AggregationCount,
			Variable: aggCountMatch[1],
			Alias:    aggCountMatch[2],
		}
	}

	aggSumMatch := regexp.MustCompile(`(?i)SUM\s*\(\s*(\?\w+)\s*\)\s*AS\s+(\w+)`).FindStringSubmatch(query)
	if aggSumMatch != nil {
		modifier.Aggregation = &AggregationSpec{
			Type:     AggregationSum,
			Variable: aggSumMatch[1],
			Alias:    aggSumMatch[2],
		}
	}

	aggMinMatch := regexp.MustCompile(`(?i)MIN\s*\(\s*(\?\w+)\s*\)\s*AS\s+(\w+)`).FindStringSubmatch(query)
	if aggMinMatch != nil {
		modifier.Aggregation = &AggregationSpec{
			Type:     AggregationMin,
			Variable: aggMinMatch[1],
			Alias:    aggMinMatch[2],
		}
	}

	aggMaxMatch := regexp.MustCompile(`(?i)MAX\s*\(\s*(\?\w+)\s*\)\s*AS\s+(\w+)`).FindStringSubmatch(query)
	if aggMaxMatch != nil {
		modifier.Aggregation = &AggregationSpec{
			Type:     AggregationMax,
			Variable: aggMaxMatch[1],
			Alias:    aggMaxMatch[2],
		}
	}

	baseQuery := regexp.MustCompile(`(?i)(?:LIMIT|OFFSET|ORDER\s+BY|GROUP\s+BY|COUNT\(|SUM\(|MIN\(|MAX\().*`).ReplaceAllString(query, "")
	baseQuery = strings.TrimSpace(baseQuery)

	return modifier, baseQuery
}

func (pq *ParsedQuery) BaseQuery() string {
	var sb strings.Builder
	for i, atom := range pq.Atoms {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(atom.Predicate)
		sb.WriteString("(")
		for j, arg := range atom.Args {
			if j > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(arg)
		}
		sb.WriteString(")")
	}
	return sb.String()
}

func (pq *ParsedQuery) String() string {
	var sb strings.Builder
	sb.WriteString(pq.BaseQuery())

	if pq.Modifier != nil {
		if len(pq.Modifier.GroupBy) > 0 {
			sb.WriteString(" GROUP BY ")
			sb.WriteString(strings.Join(pq.Modifier.GroupBy, ", "))
		}

		if pq.Modifier.Aggregation != nil {
			sb.WriteString(" ")
			switch pq.Modifier.Aggregation.Type {
			case AggregationCount:
				sb.WriteString(fmt.Sprintf("COUNT(%s) AS %s", pq.Modifier.Aggregation.Variable, pq.Modifier.Aggregation.Alias))
			case AggregationSum:
				sb.WriteString(fmt.Sprintf("SUM(%s) AS %s", pq.Modifier.Aggregation.Variable, pq.Modifier.Aggregation.Alias))
			case AggregationMin:
				sb.WriteString(fmt.Sprintf("MIN(%s) AS %s", pq.Modifier.Aggregation.Variable, pq.Modifier.Aggregation.Alias))
			case AggregationMax:
				sb.WriteString(fmt.Sprintf("MAX(%s) AS %s", pq.Modifier.Aggregation.Variable, pq.Modifier.Aggregation.Alias))
			}
		}

		if len(pq.Modifier.SortBy) > 0 {
			sb.WriteString(" ORDER BY ")
			for i, s := range pq.Modifier.SortBy {
				if i > 0 {
					sb.WriteString(", ")
				}
				sb.WriteString(s.Variable)
				if s.Direction == SortDesc {
					sb.WriteString(" DESC")
				}
			}
		}

		if pq.Modifier.Limit != nil {
			sb.WriteString(fmt.Sprintf(" LIMIT %d", *pq.Modifier.Limit))
		}

		if pq.Modifier.Offset != nil {
			sb.WriteString(fmt.Sprintf(" OFFSET %d", *pq.Modifier.Offset))
		}
	}

	return sb.String()
}

func ApplyModifiers(results []map[string]any, modifier *QueryModifier) []map[string]any {
	if modifier == nil {
		return results
	}

	var filtered []map[string]any

	if len(modifier.GroupBy) > 0 && modifier.Aggregation != nil {
		filtered = applyAggregation(results, modifier)
	} else {
		filtered = results
	}

	if len(modifier.SortBy) > 0 {
		filtered = applySort(filtered, modifier.SortBy)
	}

	if modifier.Offset != nil && *modifier.Offset > 0 {
		if *modifier.Offset >= len(filtered) {
			return []map[string]any{}
		}
		filtered = filtered[*modifier.Offset:]
	}

	if modifier.Limit != nil && *modifier.Limit > 0 {
		if *modifier.Limit < len(filtered) {
			filtered = filtered[:*modifier.Limit]
		}
	}

	return filtered
}

func applyAggregation(results []map[string]any, modifier *QueryModifier) []map[string]any {
	if len(modifier.GroupBy) == 0 {
		return results
	}

	groups := make(map[string][]map[string]any)

	for _, r := range results {
		var keyParts []string
		for _, gVar := range modifier.GroupBy {
			if val, ok := r[gVar]; ok {
				keyParts = append(keyParts, fmt.Sprintf("%v", val))
			}
		}
		key := strings.Join(keyParts, "|")
		groups[key] = append(groups[key], r)
	}

	var aggregated []map[string]any
	for _, group := range groups {
		aggResult := make(map[string]any)

		for _, gVar := range modifier.GroupBy {
			if len(group) > 0 {
				if val, ok := group[0][gVar]; ok {
					aggResult[gVar] = val
				}
			}
		}

		if modifier.Aggregation != nil {
			alias := "?" + modifier.Aggregation.Alias
			switch modifier.Aggregation.Type {
			case AggregationCount:
				aggResult[alias] = len(group)
			case AggregationSum:
				var sum float64
				for _, r := range group {
					if val, ok := r[modifier.Aggregation.Variable]; ok {
						if f, ok := toFloat64(val); ok {
							sum += f
						}
					}
				}
				aggResult[alias] = sum
			case AggregationMin:
				var min float64 = 1<<63 - 1
				for _, r := range group {
					if val, ok := r[modifier.Aggregation.Variable]; ok {
						if f, ok := toFloat64(val); ok {
							if f < min {
								min = f
							}
						}
					}
				}
				aggResult[alias] = min
			case AggregationMax:
				var max float64 = -1 << 63
				for _, r := range group {
					if val, ok := r[modifier.Aggregation.Variable]; ok {
						if f, ok := toFloat64(val); ok {
							if f > max {
								max = f
							}
						}
					}
				}
				aggResult[alias] = max
			}
		}

		aggregated = append(aggregated, aggResult)
	}

	return aggregated
}

func applySort(results []map[string]any, sortSpecs []SortSpec) []map[string]any {
	if len(results) == 0 || len(sortSpecs) == 0 {
		return results
	}

	sorted := make([]map[string]any, len(results))
	copy(sorted, results)

	sort.Slice(sorted, func(i, j int) bool {
		for _, spec := range sortSpecs {
			val1 := sorted[i][spec.Variable]
			val2 := sorted[j][spec.Variable]

			cmp := compareValues(val1, val2)
			if spec.Direction == SortDesc {
				cmp = -cmp
			}

			if cmp < 0 {
				return true
			} else if cmp > 0 {
				return false
			}
		}
		return false
	})

	return sorted
}

func toFloat64(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func compareValues(a, b any) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}

	af, aok := toFloat64(a)
	bf, bok := toFloat64(b)

	if aok && bok {
		if af < bf {
			return -1
		} else if af > bf {
			return 1
		}
		return 0
	}

	as := fmt.Sprintf("%v", a)
	bs := fmt.Sprintf("%v", b)

	if as < bs {
		return -1
	} else if as > bs {
		return 1
	}
	return 0
}
