package mpq

import (
	"bytes"
	"io/ioutil"
	"path"
	"strings"
	"testing"
	"time"
)

func TestSpeed(t *testing.T) {
	const N = 10
	files := []string{
		"replay.details",
		"replay.initData",
		"replay.attributes.events",
		"replay.message.events",
		"replay.game.events",
		"replay.tracker.events",
		"(attributes)",
	}

	start := time.Now()
	for i := 0; i < N; i++ {
		m, _ := NewFromFile("reps/automm.SC2Replay")

		for _, file := range files {
			m.FileByName(file)
		}
		m.Close()
	}
	println(time.Since(start)/1000000/N, "ms")
}

func TestReps(t *testing.T) {
	testFolder(t, "reps")
}

func testFolder(t *testing.T, folder string) {
	fis, err := ioutil.ReadDir(folder)
	if err != nil {
		t.Errorf("Can't read folder: %s, error: %v", folder, err)
		return
	}

	for _, fi := range fis {
		if fi.IsDir() || !strings.HasSuffix(fi.Name(), ".SC2Replay") {
			continue
		}
		name := path.Join(folder, fi.Name())

		testFile(t, name)
	}
}

func testFile(t *testing.T, name string) {
	// 2 rounds: from file and from memory
	for i := 0; i < 2; i++ {
		var m *MPQ
		var err error

		if i == 0 {
			m, err = NewFromFile(name)
		} else {
			var content []byte
			content, err = ioutil.ReadFile(name)
			if err != nil {
				t.Errorf("Failed to read input file: %s, error: %v", name, err)
			}

			m, err = New(bytes.NewReader(content))
			if err != nil {
				t.Errorf("Failed to read from memory buffer, error: %v", err)
			}
		}

		if err != nil {
			t.Errorf("Can't parse MPQ: %s, error: %v", name, err)
			return
		}

		func() {
			defer m.Close()
			testMpq(t, name, m)
		}()
	}
}

func testMpq(t *testing.T, name string, m *MPQ) {
	files := []string{
		"replay.details",
		"replay.initData",
		"replay.attributes.events",
		"replay.message.events",
		"replay.game.events",
		"replay.tracker.events",
		"replay.sync.events",
		"replay.smartcam.events",
		"replay.load.info",
		"replay.resumable.events",
		"replay.server.battlelobby",
		"(attributes)",
		"(listfile)",
	}
	for _, file := range files {
		if _, err := m.FileByName(file); err != nil {
			t.Errorf("Error getting file '%s' from archive: %s, error: %v", name, file, err)
		}
	}
}

func TestInvalid(t *testing.T) {
	names := []string{
		"reps/invalid.SC2Replay_",
		"I-DONT-EXIST.SC2Replay",
	}
	for _, name := range names {
		m, err := NewFromFile(name)
		if m != nil || err == nil {
			t.Errorf("Parse should have failed but it succeeded: %s, error: %v", name, err)
		}
	}

	m, err := New(bytes.NewReader([]byte("INVALID")))
	if m != nil || err == nil {
		t.Errorf("Parse should have failed but it succeeded: error: %v", err)
	}
}
