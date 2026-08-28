package audit

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var _ io.Closer = (*fileObserver)(nil)

func TestNewFileObserver_CreatesFileAndDirectories(t *testing.T) {
	base := t.TempDir()
	nested := filepath.Join(base, "a", "b", "audit.log")

	obs, err := newFileObserver(nested)
	require.NoError(t, err)
	require.NotNil(t, obs)

	_, statErr := os.Stat(nested)
	require.NoError(t, statErr)
	assert.Equal(t, nested, obs.Name())
}

func TestFileObserver_Notify_AppendsJSONLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	obs, err := newFileObserver(path)
	require.NoError(t, err)

	event := NewEvent([]string{"cpu"}, "1.2.3.4")
	require.NoError(t, obs.Notify(event))

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	require.Len(t, lines, 1)
	assert.True(t, strings.HasSuffix(string(data), "\n"))

	var got Event
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &got))
	assert.Equal(t, event, got)
}

func TestFileObserver_Notify_MultipleSequentialCalls(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	obs, err := newFileObserver(path)
	require.NoError(t, err)

	want := []Event{
		NewEvent([]string{"cpu"}, "1.1.1.1"),
		NewEvent([]string{"hits"}, "2.2.2.2"),
		NewEvent([]string{"temp", "mem"}, "3.3.3.3"),
	}
	for _, e := range want {
		require.NoError(t, obs.Notify(e))
	}

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	var got []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e Event
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &e))
		got = append(got, e)
	}
	require.NoError(t, scanner.Err())

	assert.Equal(t, want, got)
}

func TestFileObserver_Notify_Concurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	obs, err := newFileObserver(path)
	require.NoError(t, err)

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_ = obs.Notify(NewEvent([]string{"metric"}, "1.1.1.1"))
		}(i)
	}
	wg.Wait()

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	lineCount := 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e Event
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &e))
		lineCount++
	}
	require.NoError(t, scanner.Err())

	assert.Equal(t, n, lineCount)
}

func TestFileObserver_Close(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	obs, err := newFileObserver(path)
	require.NoError(t, err)

	assert.NoError(t, obs.Close())
}

func TestFileObserver_Notify_WriteError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.log")

	obs, err := newFileObserver(path)
	require.NoError(t, err)
	require.NoError(t, obs.file.Close())

	err = obs.Notify(NewEvent([]string{"cpu"}, "1.1.1.1"))
	assert.Error(t, err)
}
