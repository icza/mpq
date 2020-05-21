# mpq

[![Build Status](https://travis-ci.org/icza/mpq.svg?branch=master)](https://travis-ci.org/icza/mpq)
[![GoDoc](https://godoc.org/github.com/icza/mpq?status.svg)](https://godoc.org/github.com/icza/mpq)
[![Go Report Card](https://goreportcard.com/badge/github.com/icza/mpq)](https://goreportcard.com/report/github.com/icza/mpq)
[![codecov](https://codecov.io/gh/icza/mpq/branch/master/graph/badge.svg)](https://codecov.io/gh/icza/mpq)

Package `mpq` is a decoder/parser of Blizzard's MPQ archive file format.

This is not a full MPQ implementation. It is primarily intended to parse StarCraft II replay files (`*.SC2Replay`),
but that is fully supported.

## Usage

Usage is simple. Opening an MPQ archive file:

	m, err := mpq.NewFromFile("myreplay.SC2Replay")
	if err != nil {
		// Handle error
		return
	}
	defer m.Close()

Getting a named file from the archive:

	// Access a file inside the MPQ archive.
	// Usually there is a file called "(listfile)" containing the list of other files:
	if data, err := m.FileByName("(listfile)"); err == nil {
		fmt.Println("Files inside archive:")
		fmt.Println(string(data))
	} else {
		// handle error
	}

If you already have the MPQ data in memory:

	mpqdata := []byte{} // MPQ data in memory
	m, err := mpq.New(bytes.NewReader(mpqdata)))

## Information sources

- The_MoPaQ_Archive_Format: http://wiki.devklog.net/index.php?title=The_MoPaQ_Archive_Format
- MPQ on wikipedia: http://en.wikipedia.org/wiki/MPQ
- Zezula MPQ description: http://www.zezula.net/mpq.html
- Stormlib: https://github.com/ladislav-zezula/StormLib
- Libmpq project: https://github.com/ge0rg/libmpq (old: https://libmpq.org/)
- MPQ parser of the Scelight project: https://github.com/icza/scelight/tree/master/src-app-libs/hu/belicza/andras/mpq

## Example projects using this

- https://github.com/icza/s2prot

## License

Open-sourced under the [Apache License 2.0](https://github.com/icza/mpq/blob/master/LICENSE).
