package cronexpr

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImpossibleCalendarDateNeverMatchesOrLoops(t *testing.T) {
	schedule, err := Parse("0 0 31 2 *")
	require.NoError(t, err)
	for year := 2024; year <= 2025; year++ {
		start := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(1, 0, 0)
		for current := start; current.Before(end); current = current.AddDate(0, 0, 1) {
			assert.False(t, schedule.Matches(current), current.String())
		}
	}
}

func TestLeapDayAndCronDayWeekORSemantics(t *testing.T) {
	leapDay, err := Parse("0 0 29 2 *")
	require.NoError(t, err)
	assert.True(t, leapDay.Matches(time.Date(2024, time.February, 29, 0, 0, 0, 0, time.UTC)))
	assert.False(t, leapDay.Matches(time.Date(2025, time.February, 28, 0, 0, 0, 0, time.UTC)))

	dayOrMonday, err := Parse("0 0 31 2 1")
	require.NoError(t, err)
	assert.True(t, dayOrMonday.Matches(time.Date(2025, time.February, 3, 0, 0, 0, 0, time.UTC)))
}
