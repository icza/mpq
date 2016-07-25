# mpq

[![GoDoc](https://godoc.org/github.com/icza/mpq?status.svg)](https://godoc.org/github.com/icza/mpq)

Package `mpq` is a decoder/parser of Blizzard's MPQ archive file format.

This is not a full implementation, primarily intended to parse StarCraft II replay files (`*.SC2Replay`).

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

	mpqdata := []byte{/* ... */} // MPQ data in memory
	m, err := mpq.New(bytes.NewReader(mpqdata)))

## Information sources

- The_MoPaQ_Archive_Format: http://wiki.devklog.net/index.php?title=The_MoPaQ_Archive_Format
- MPQ on wikipedia: http://en.wikipedia.org/wiki/MPQ
- Zezula MPQ description: http://www.zezula.net/mpq.html
- Stormlib: https://github.com/ladislav-zezula/StormLib
- Libmpq project: https://github.com/ge0rg/libmpq (old: https://libmpq.org/)
- MPQ parser of the Scelight project: https://github.com/icza/scelight/tree/master/src-app-libs/hu/belicza/andras/mpq

## License

Open-sourced under the [Apache License 2.0](https://github.com/icza/scelight/blob/master/LICENSE).
