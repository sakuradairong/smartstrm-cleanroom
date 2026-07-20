package tmdb

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderTemplate(t *testing.T) {
	values := map[string]string{"title": "Demo", "season": "1", "episode": "2", "episode_name": "Pilot", "ext": ".mkv"}
	name, err := Render(`{title} - S{season:2}E{episode:3}{.,episode_name}{ext}`, values)
	require.NoError(t, err)
	assert.Equal(t, "Demo - S01E002.Pilot.mkv", name)
	delete(values, "episode_name")
	name, err = Render(`S{season:2}E{episode:2}{.,episode_name}{ext}`, values)
	require.NoError(t, err)
	assert.Equal(t, "S01E02.mkv", name)
}

func TestRenderTemplateRejectsUnsafeOutput(t *testing.T) {
	_, err := Render(`{title}`, map[string]string{"title": "../escape"})
	require.Error(t, err)
	_, err = Render(`{episode:nope}`, map[string]string{"episode": "1"})
	require.Error(t, err)
}
