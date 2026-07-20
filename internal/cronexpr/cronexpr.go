package cronexpr

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Schedule struct {
	fields   [5]map[int]bool
	wildcard [5]bool
}

func Parse(value string) (Schedule, error) {
	fields := strings.Fields(value)
	if len(fields) != 5 {
		return Schedule{}, fmt.Errorf("expected five-field crontab")
	}
	ranges := [5][2]int{{0, 59}, {0, 23}, {1, 31}, {1, 12}, {0, 6}}
	names := [5]string{"minute", "hour", "day", "month", "weekday"}
	var result Schedule
	for index, field := range fields {
		parsed, err := parseField(field, ranges[index][0], ranges[index][1])
		if err != nil {
			return Schedule{}, fmt.Errorf("%s: %w", names[index], err)
		}
		result.fields[index] = parsed
		result.wildcard[index] = strings.HasPrefix(field, "*")
	}
	return result, nil
}

func parseField(value string, minimum, maximum int) (map[int]bool, error) {
	allowed := make(map[int]bool)
	for _, item := range strings.Split(value, ",") {
		base, stepText, hasStep := strings.Cut(item, "/")
		step := 1
		var err error
		if hasStep {
			step, err = strconv.Atoi(stepText)
			if err != nil || step < 1 {
				return nil, fmt.Errorf("invalid step %q", stepText)
			}
		}
		start, end := minimum, maximum
		if base != "*" {
			if left, right, hasRange := strings.Cut(base, "-"); hasRange {
				start, err = strconv.Atoi(left)
				if err != nil {
					return nil, fmt.Errorf("invalid range %q", base)
				}
				end, err = strconv.Atoi(right)
				if err != nil {
					return nil, fmt.Errorf("invalid range %q", base)
				}
			} else {
				start, err = strconv.Atoi(base)
				if err != nil {
					return nil, fmt.Errorf("invalid value %q", base)
				}
				end = start
			}
		}
		if start < minimum || end > maximum || start > end {
			return nil, fmt.Errorf("values must be within %d-%d", minimum, maximum)
		}
		for number := start; number <= end; number += step {
			allowed[number] = true
		}
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("field has no values")
	}
	return allowed, nil
}

func (s Schedule) Matches(value time.Time) bool {
	values := [5]int{value.Minute(), value.Hour(), value.Day(), int(value.Month()), int(value.Weekday())}
	if !s.fields[0][values[0]] || !s.fields[1][values[1]] || !s.fields[3][values[3]] {
		return false
	}
	dayMatches, weekdayMatches := s.fields[2][values[2]], s.fields[4][values[4]]
	switch {
	case s.wildcard[2] && s.wildcard[4]:
		return true
	case s.wildcard[2]:
		return weekdayMatches
	case s.wildcard[4]:
		return dayMatches
	default:
		return dayMatches || weekdayMatches
	}
}
