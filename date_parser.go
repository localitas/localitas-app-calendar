package calendar

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

func ParseNaturalDate(input string) (time.Time, bool) {
	input = strings.TrimSpace(strings.ToLower(input))
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)

	switch input {
	case "today":
		return today, true
	case "tomorrow":
		return today.AddDate(0, 0, 1), true
	case "yesterday":
		return today.AddDate(0, 0, -1), true
	}

	if strings.HasPrefix(input, "next ") {
		dayName := strings.TrimPrefix(input, "next ")
		if weekday, ok := parseWeekday(dayName); ok {
			return nextWeekday(today, weekday), true
		}
		if month, ok := parseMonth(dayName); ok {
			return nextMonth(today, month), true
		}
	}

	if strings.HasPrefix(input, "last ") {
		dayName := strings.TrimPrefix(input, "last ")
		if weekday, ok := parseWeekday(dayName); ok {
			return lastWeekday(today, weekday), true
		}
	}

	if weekday, ok := parseWeekday(input); ok {
		return nextWeekday(today, weekday), true
	}

	// Remove ordinal suffixes (1st, 2nd, 3rd, 4th, etc.)
	normalized := removeOrdinalSuffix(input)

	formats := []string{
		"2006-01-02",
		"01/02/2006",
		"1/2/2006",
		"01-02-2006",
		"Jan 2, 2006",
		"Jan 2 2006",
		"January 2, 2006",
		"January 2 2006",
		"2 Jan 2006",
		"2 January 2006",
	}

	for _, format := range formats {
		if t, err := time.ParseInLocation(format, normalized, time.Local); err == nil {
			return t, true
		}
	}

	shortFormats := []string{
		"Jan 2",
		"Jan 02",
		"January 2",
		"January 02",
		"1/2",
		"01/02",
	}

	for _, format := range shortFormats {
		if t, err := time.ParseInLocation(format, normalized, time.Local); err == nil {
			result := time.Date(now.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
			if result.Before(today) {
				result = result.AddDate(1, 0, 0)
			}
			return result, true
		}
	}

	relativePattern := regexp.MustCompile(`^(\d+)\s*(days?|weeks?|months?)\s*(ago|from now|later)?$`)
	if matches := relativePattern.FindStringSubmatch(input); matches != nil {
		num, _ := strconv.Atoi(matches[1])
		unit := matches[2]
		direction := matches[3]

		if direction == "ago" {
			num = -num
		}

		switch {
		case strings.HasPrefix(unit, "day"):
			return today.AddDate(0, 0, num), true
		case strings.HasPrefix(unit, "week"):
			return today.AddDate(0, 0, num*7), true
		case strings.HasPrefix(unit, "month"):
			return today.AddDate(0, num, 0), true
		}
	}

	return time.Time{}, false
}

func removeOrdinalSuffix(input string) string {
	ordinalPattern := regexp.MustCompile(`(\d+)(st|nd|rd|th)`)
	return ordinalPattern.ReplaceAllString(input, "$1")
}

func parseWeekday(name string) (time.Weekday, bool) {
	weekdays := map[string]time.Weekday{
		"sunday":    time.Sunday,
		"sun":       time.Sunday,
		"monday":    time.Monday,
		"mon":       time.Monday,
		"tuesday":   time.Tuesday,
		"tue":       time.Tuesday,
		"tues":      time.Tuesday,
		"wednesday": time.Wednesday,
		"wed":       time.Wednesday,
		"thursday":  time.Thursday,
		"thu":       time.Thursday,
		"thurs":     time.Thursday,
		"friday":    time.Friday,
		"fri":       time.Friday,
		"saturday":  time.Saturday,
		"sat":       time.Saturday,
	}
	wd, ok := weekdays[name]
	return wd, ok
}

func parseMonth(name string) (time.Month, bool) {
	months := map[string]time.Month{
		"january":   time.January,
		"jan":       time.January,
		"february":  time.February,
		"feb":       time.February,
		"march":     time.March,
		"mar":       time.March,
		"april":     time.April,
		"apr":       time.April,
		"may":       time.May,
		"june":      time.June,
		"jun":       time.June,
		"july":      time.July,
		"jul":       time.July,
		"august":    time.August,
		"aug":       time.August,
		"september": time.September,
		"sep":       time.September,
		"sept":      time.September,
		"october":   time.October,
		"oct":       time.October,
		"november":  time.November,
		"nov":       time.November,
		"december":  time.December,
		"dec":       time.December,
	}
	m, ok := months[name]
	return m, ok
}

func nextWeekday(from time.Time, target time.Weekday) time.Time {
	daysUntil := int(target) - int(from.Weekday())
	if daysUntil <= 0 {
		daysUntil += 7
	}
	return from.AddDate(0, 0, daysUntil)
}

func lastWeekday(from time.Time, target time.Weekday) time.Time {
	daysSince := int(from.Weekday()) - int(target)
	if daysSince <= 0 {
		daysSince += 7
	}
	return from.AddDate(0, 0, -daysSince)
}

func nextMonth(from time.Time, target time.Month) time.Time {
	year := from.Year()
	if from.Month() >= target {
		year++
	}
	return time.Date(year, target, 1, 0, 0, 0, 0, time.Local)
}
