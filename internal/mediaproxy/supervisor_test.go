package mediaproxy

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	return address
}

func get(t *testing.T, address string) (int, string) {
	t.Helper()
	response, err := http.Get("http://" + address + "/probe")
	require.NoError(t, err)
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	return response.StatusCode, string(body)
}

func TestSupervisorSwapsUpstreamWithoutRestart(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "first")
	}))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "second")
	}))
	defer second.Close()

	address := freeAddress(t)
	supervisor := NewSupervisor()
	t.Cleanup(supervisor.Stop)

	restart, err := supervisor.Apply(config.MediaProxyConfig{Enabled: true, Listen: address, Upstream: first.URL}, "", "")
	require.NoError(t, err)
	require.False(t, restart)
	status, body := get(t, address)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "first", body)

	// Point at a different upstream; the listener must stay bound.
	restart, err = supervisor.Apply(config.MediaProxyConfig{Enabled: true, Listen: address, Upstream: second.URL}, "", "")
	require.NoError(t, err)
	assert.False(t, restart, "changing the upstream must not require a restart")
	status, body = get(t, address)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, "second", body, "requests must reach the new upstream")
}

func TestSupervisorStartsAndStopsOnEnabledFlag(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer upstream.Close()
	address := freeAddress(t)
	supervisor := NewSupervisor()
	t.Cleanup(supervisor.Stop)

	// Disabled: nothing is listening.
	_, err := supervisor.Apply(config.MediaProxyConfig{Enabled: false}, "", "")
	require.NoError(t, err)
	assert.False(t, supervisor.Running())
	_, err = http.Get("http://" + address + "/probe")
	assert.Error(t, err, "a disabled proxy must not hold the port")

	// Enable it live.
	_, err = supervisor.Apply(config.MediaProxyConfig{Enabled: true, Listen: address, Upstream: upstream.URL}, "", "")
	require.NoError(t, err)
	assert.True(t, supervisor.Running())
	status, _ := get(t, address)
	assert.Equal(t, http.StatusOK, status)

	// Disable it again and confirm the port is released.
	_, err = supervisor.Apply(config.MediaProxyConfig{Enabled: false}, "", "")
	require.NoError(t, err)
	assert.False(t, supervisor.Running())
	listener, err := net.Listen("tcp", address)
	require.NoError(t, err, "port must be free after the proxy stops")
	require.NoError(t, listener.Close())
}

func TestSupervisorReportsRestartOnlyForListenChange(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer upstream.Close()
	address := freeAddress(t)
	supervisor := NewSupervisor()
	t.Cleanup(supervisor.Stop)

	_, err := supervisor.Apply(config.MediaProxyConfig{Enabled: true, Listen: address, Upstream: upstream.URL}, "", "")
	require.NoError(t, err)

	restart, err := supervisor.Apply(config.MediaProxyConfig{Enabled: true, Listen: freeAddress(t), Upstream: upstream.URL}, "", "")
	require.NoError(t, err)
	assert.True(t, restart, "a new listening address needs a restart")
	assert.Equal(t, address, supervisor.Listen(), "the original port stays bound until restart")
}

func TestSupervisorRejectsInvalidConfigurationWithoutDroppingProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer upstream.Close()
	address := freeAddress(t)
	supervisor := NewSupervisor()
	t.Cleanup(supervisor.Stop)

	_, err := supervisor.Apply(config.MediaProxyConfig{Enabled: true, Listen: address, Upstream: upstream.URL}, "", "")
	require.NoError(t, err)

	_, err = supervisor.Apply(config.MediaProxyConfig{Enabled: true, Listen: address, Upstream: "not-a-url"}, "", "")
	require.Error(t, err, "an invalid upstream must be rejected")
	status, body := get(t, address)
	assert.Equal(t, http.StatusOK, status, "the running proxy must survive a rejected change")
	assert.Equal(t, "ok", body)
}

func TestSupervisorPreparedChangeCanAbortWithoutPublishing(t *testing.T) {
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "first") }))
	defer first.Close()
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, "second") }))
	defer second.Close()
	s := NewSupervisor()
	t.Cleanup(s.Stop)
	address := freeAddress(t)
	_, err := s.Apply(config.MediaProxyConfig{Enabled: true, Listen: address, Upstream: first.URL}, "", "")
	require.NoError(t, err)
	_, abort, err := s.Prepare(config.MediaProxyConfig{Enabled: true, Listen: address, Upstream: second.URL}, "", "")
	require.NoError(t, err)
	// ServeHTTP reads the active handler without locking Prepare's mutex.
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, httptest.NewRequest("GET", "http://example.test/probe", nil))
	require.Equal(t, "first", rr.Body.String())
	abort()
	_, body := get(t, address)
	require.Equal(t, "first", body)

	s.Stop()
	_, abort, err = s.Prepare(config.MediaProxyConfig{Enabled: true, Listen: address, Upstream: second.URL}, "", "")
	require.NoError(t, err)
	abort()
	listener, err := net.Listen("tcp", address)
	require.NoError(t, err, "aborted preparation must release its reserved listener")
	require.NoError(t, listener.Close())
}
