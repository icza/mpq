// Implementation note:
// In this implementation I read structs from the MPQ source field-by-field for efficiency
// because fields are primitive types and binary.Read() is optimized for them
// and hence no reflection will be applied.

package mpq

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

var (
	// ErrInvalidArchive indicates an invalid MPQ archive
	ErrInvalidArchive = errors.New("Invalid MPQ Archive")
)

// blockEntry.flag bitmask constants.
const (
	// Flag indicating that block is a file, and follows the file data format; otherwise, block is free space or unused.
	beFlagFile = 0x80000000

	// Flag indicating that file is stored as a single unit, rather than split into sectors.
	beFlagSingle = 0x01000000

	//Flag indicating that the file has checksums for each sector (explained in the File Data section). Ignored if file is not compressed or imploded.
	beFlagExtra = 0x04000000

	// Flag indicating that the file is compressed.
	beFlagCompressed = 0x0000FF00

	// Flag indicating that the file is compressed with pkware algorithm.
	beFlagPKWare = 0x00000100

	// Flag indicating that the file is under multiple compression.
	beFlagCompressedMulti = 0x00000200

	// Flag indicating that the file is encrypted.
	beFlagEncrypted = 0x00010000
)

// The User Data before the header of the MPQ archives.
//
// The second version of the MoPaQ format, first used in Burning Crusade, features a mechanism to store
// some amount of data outside the archive proper, though the reason for this mechanism is not known.
// This is implemented by means of a shunt block that precedes the archive itself.
// The format of this block is as follows (see below):
//
// When Storm encounters this block in its search for the archive header, it saves
// the location of the shunt block and resumes its search for the archive header
// at the offset specified in the shunt.
//
// Blizzard-generated archives place the shunt at the beginning of the file, and begin
// the archive itself at the next 512-byte boundary after the end of the shunt block.
type userData struct {
	// The number of bytes that have been allocated in this archive for user data.
	// This does not need to be the exact size of the data itself,
	// but merely the maximum amount of data which may be stored in this archive.
	size uint32

	// The offset in the file at which to continue the search for the archive header.
	headerOffset uint32

	// The block to store user data in. It has a length of Size.
	data []byte // User data
}

// The header of the MPQ archives.
//
// The archive header is the first structure in the archive, at archive offset 0;
// however, the archive does not need to be at offset 0 of the containing file.
// The offset of the archive in the file is referred to here as ArchiveOffset.
// If the archive is not at the beginning of the file, it must begin at a disk
// sector boundary (512 bytes). Early versions of Storm require that the archive
// be at the end of the containing file (ArchiveOffset + ArchiveSize = file size),
// but this is not required in newer versions
// (due to the strong digital signature not being considered a part of the archive).
type header struct {
	// Size of the archive header.
	size uint32

	// Size of the whole archive, including the header. Does not include the strong digital signature,
	// if present. This size is used, among other things, for determining the region to hash
	// in computing the digital signature. This field is deprecated in the Burning Crusade MoPaQ format,
	// and the size of the archive is calculated as the size from the beginning of the archive
	// to the end of the hash table, block table, or extended block table (whichever is largest).
	archiveSize uint32

	// MoPaQ format version. MPQAPI will not open archives where this is negative. Known versions:
	//     0x0000 Original format. HeaderSize should be 20h, and large archives are not supported.
	//     0x0001 Burning Crusade format. Header size should be 2Ch, and large archives are supported.
	formatVersion uint16

	// Power of two exponent specifying the number of 512-byte disk sectors in each logical sector
	// in the archive. The size of each logical sector in the archive is 512 * 2^SectorSizeShift.
	// Bugs in the Storm library dictate that this should always be 3 (4096 byte sectors).
	sectorSizeShift uint16

	// Offset to the beginning of the hash table, relative to the beginning of the archive.
	hashTableOffset uint32

	// Offset to the beginning of the block table, relative to the beginning of the archive.
	blockTableOffset uint32

	// Number of entries in the hash table. Must be a power of two, and must be less than 2^16
	// for the original MoPaQ format, or less than 2^20 for the Burning Crusade format.
	hashTableEntries uint32

	// Number of entries in the block table.
	blockTableEntries uint32

	// Fields only present in the Burning Crusade format and later (FormatVersion > 0):

	// Offset to the beginning of the extended block table, relative to the beginning of the archive.
	extendedBlockTableOffset uint64

	// High 16 bits of the hash table offset for large archives.
	hashTableOffsetHigh uint16

	// High 16 bits of the block table offset for large archives.
	blockTableOffsetHigh uint16

	// Note: in FormatVersion > 1 there are further fields which I do not implement/use.
}

// Entries of the Hash table section of the MPQ archives.
//
// Instead of storing file names, for quick access MoPaQs use a fixed, power of two-size
// hash table of files in the archive. A file is uniquely identified by its file path,
// its language, and its platform.
//
// The home entry for a file in the hash table is computed as a hash of the file path.
// In the event of a collision (the home entry is occupied by another file),
// progressive overflow is used, and the file is placed in the next available hash table entry.
// Searches for a desired file in the hash table proceed from the home entry for the file
// until either the file is found, the entire hash table is searched, or an empty hash table entry
// (fileBlockIndex of 0xFFFFFFFF) is encountered.
//
// The hash table is always encrypted, using the hash of "(hash table)" as the key.
//
// Prior to Starcraft 2, the hash table was stored uncompressed. In Starcraft 2, however,
// the table may optionally be compressed. If the offset of the block table is not equal
// to the offset of the hash table plus the uncompressed size, Starcraft 2 interprets the hash table
// as being compressed (not imploded). This calculation assumes that the block table immediately
// follows the hash table, and will fail or crash otherwise.
type hashEntry struct {
	// The hash of the file path, using method A.
	filePathHashA uint32

	// The hash of the file path, using method B.
	filePathHashB uint32

	// The language of the file. This is a Windows LANGID data type, and uses the same values.
	// 0 indicates the default language (American English), or that the file is language-neutral.
	language uint16

	// The platform the file is used for. 0 indicates the default platform. No other values have been observed.
	platform uint16

	// If the hash table entry is valid, this is the index into the block table of the file. Otherwise, one of the following two values:
	//     0xffffffff Hash table entry is empty, and has always been empty. Terminates searches for a given file.
	//     0xfffffffe Hash table entry is empty, but was valid at some point (in other words, the file was deleted).
	//                Does not terminate searches for a given file.
	fileBlockIndex uint32
}

// Entries of the Block table section of the MPQ archives.
//
// The block table contains entries for each region in the archive. Regions may be either files,
// empty space, which may be overwritten by new files (typically this space is from deleted file data),
// or unused block table entries. Empty space entries should have BlockOffset and BlockSize nonzero,
// and FileSize and Flags zero; unused block table entries should have BlockSize, FileSize, and Flags zero.
// The block table is encrypted, using the hash of "(block table)" as the key.
//
// Extended block table
//
// The extended block table was added to support archives larger than 4 gigabytes (2^32 bytes).
// The table contains the upper bits of the archive offsets for each block in the block table.
// It is simply an array of uint16, which become bits 32-47 of the archive offsets for each block,
// with bits 48-63 being zero. Individual blocks in the archive are still limited to 4 gigabytes in size.
// This table is only present in Burning Crusade format archives that exceed 4 gigabytes size.
//
// Unlike the hash and block tables, the extended block table is not encrypted nor compressed.
type blockEntry struct {
	// Offset of the beginning of the block, relative to the beginning of the archive.
	blockOffset uint32

	// Size of the block in the archive. Also referred to as packedSize.
	blockSize uint32

	// Size of the file data stored in the block. Only valid if the block is a file;
	// otherwise meaningless, and should be 0. If the file is compressed, this is the size
	// of the uncompressed file data. Also referred to as unpackedSize.
	fileSize uint32

	// Bit mask of the flags for the block. The following values are conclusively identified:
	//     0x80000000 Block is a file, and follows the file data format; otherwise, block is free space or unused.
	//                If the block is not a file, all other flags should be cleared, and FileSize should be 0.
	//     0x04000000 File has checksums for each sector (explained in the File Data section).
	//                Ignored if file is not compressed or imploded.
	//     0x02000000 File is a deletion marker, indicating that the file no longer exists.
	//                This is used to allow patch archives to delete files present in lower-priority archives in the search chain.
	//     0x01000000 File is stored as a single unit, rather than split into sectors.
	//     0x00020000 The file's encryption key is adjusted by the block offset and file size
	//                (explained in detail in the File Data section). File must be encrypted.
	//     0x00010000 File is encrypted.
	//     0x00000200 File is compressed. File cannot be imploded.
	//     0x00000100 File is imploded. File cannot be compressed.
	flags uint32
}

// MPQ describes an MPQ archive and provides access to its content.
type MPQ struct {
	file  *os.File      // Optional source file
	input io.ReadSeeker // Input data of the MPQ content

	userData *userData // Optional UserData
	header   header    // MPQ Header

	hashTable  []hashEntry  // The Hash table
	blockTable []blockEntry // The Block table

	// The upper bits of the archive offsets for each block in the block table.
	// Only present if the archive is > 4GB.
	extBlockEntryHighOffsets []uint16

	// Derived data

	blockSize uint32 // Size of the blocks.

	blockEntryIndices []int // Block table entry indices of the files.

	filesCount uint32 // Number of files in the archive.
}

// Magic bytes of the first optional MPQ section: UserData
var userDataMagic = [4]byte{'M', 'P', 'Q', 0x1b}

// Magic bytes of the mandatory MPQHeader section
var headerMagic = [4]byte{'M', 'P', 'Q', 0x1a}

// NewFromFile returns a new MPQ using a file specified by its name as the input.
// The returned MPQ must be closed with the Close method!
// ErrInvalidArchive is returned if file exists and can be read, but is not a valid MPQ archive.
func NewFromFile(name string) (*MPQ, error) {
	var f *os.File
	var err error
	if f, err = os.Open(name); err != nil {
		return nil, err
	}

	m := &MPQ{file: f, input: f}

	return m.diveIn()
}

// New returns a new MPQ using the specified io.ReadSeeker as the input source.
// This can be used to create an MPQ out of a []byte with the help of bytes.NewReader(b []byte).
// The returned MPQ must be closed with the Close method!
// ErrInvalidArchive is returned if input is not a valid MPQ archive.
func New(input io.ReadSeeker) (*MPQ, error) {
	m := &MPQ{input: input}

	return m.diveIn()
}

// diveIn dives in into the archive data by parsing its header.
func (m *MPQ) diveIn() (*MPQ, error) {
	in := m.input

	var err error

	var magic [4]byte
	if _, err = io.ReadFull(in, magic[:]); err != nil {
		return nil, err
	}

	read := func(data interface{}) error {
		if err != nil {
			return err // No-op if we already have an error
		}
		err = binary.Read(in, binary.LittleEndian, data)
		return err
	}

	// Optionally the MPQ starts with a User Data section
	var headerOffset int64
	if magic == userDataMagic {
		u := userData{}
		read(&u.size)
		read(&u.headerOffset)
		if err == nil {
			u.data = make([]byte, u.size)
			_, err = io.ReadFull(in, u.data)
		}
		if err != nil {
			return nil, ErrInvalidArchive
		}
		m.userData = &u

		headerOffset = int64(u.headerOffset)
		if _, err = in.Seek(headerOffset, 0); err != nil { // Seek from start of the file
			return nil, ErrInvalidArchive
		}

		// Magic was UserData magic, so read the Header's magic:
		if _, err = io.ReadFull(in, magic[:]); err != nil {
			return nil, err
		}
	}

	// Check Header
	if magic != headerMagic {
		return nil, ErrInvalidArchive
	}
	h := header{}

	read(&h.size)
	read(&h.archiveSize)
	read(&h.formatVersion)
	read(&h.sectorSizeShift)
	read(&h.hashTableOffset)
	read(&h.blockTableOffset)
	read(&h.hashTableEntries)
	read(&h.blockTableEntries)

	if err != nil {
		return nil, ErrInvalidArchive
	}

	if h.formatVersion > 0 {
		read(&h.extendedBlockTableOffset)
		read(&h.hashTableOffsetHigh)
		read(&h.blockTableOffsetHigh)
	}

	if err != nil {
		return nil, ErrInvalidArchive
	}

	// Note: in FormatVersion > 1 there are further fields which I do not implement/use.

	m.header = h

	m.blockSize = 512 << h.sectorSizeShift

	// Create a big-enough buffer that is enough to read further hash and block tables to avoid reallocation:
	// Size of both hash and block entries is 16 bytes
	var buf []byte
	if h.hashTableEntries > h.blockTableEntries {
		buf = make([]byte, h.hashTableEntries*16)
	} else {
		buf = make([]byte, h.blockTableEntries*16)
	}

	// Read Hash table
	if _, err = in.Seek(int64(h.hashTableOffsetHigh)<<32+int64(h.hashTableOffset)+headerOffset, 0); err != nil {
		return nil, ErrInvalidArchive
	}
	buf = buf[:h.hashTableEntries*16]
	if _, err = io.ReadFull(in, buf); err != nil {
		return nil, ErrInvalidArchive
	}
	// Decryption key of the hash table is the value of hashString("(hash table)", hashTypeFileKey)
	decrypt(buf, 0xc3af3770)
	m.hashTable = make([]hashEntry, h.hashTableEntries)
	r := bytes.NewReader(buf)
	for i := range m.hashTable {
		he := &m.hashTable[i]
		// Reading from a byte slice whose length is "confirmed", omitting error check
		binary.Read(r, binary.LittleEndian, &he.filePathHashA)
		binary.Read(r, binary.LittleEndian, &he.filePathHashB)
		binary.Read(r, binary.LittleEndian, &he.language)
		binary.Read(r, binary.LittleEndian, &he.platform)
		binary.Read(r, binary.LittleEndian, &he.fileBlockIndex)
	}

	// Read Block table
	if _, err = in.Seek(int64(h.blockTableOffsetHigh)<<32+int64(h.blockTableOffset)+headerOffset, 0); err != nil {
		return nil, ErrInvalidArchive
	}
	buf = buf[:h.blockTableEntries*16]
	if _, err = io.ReadFull(in, buf); err != nil {
		return nil, ErrInvalidArchive
	}
	// Decryption key of the block table is the value of hashString("(block table)", hashTypeFileKey)
	decrypt(buf, 0xec83b3a3)
	m.blockTable = make([]blockEntry, h.blockTableEntries)
	r = bytes.NewReader(buf)
	for i := range m.blockTable {
		be := &m.blockTable[i]
		// Reading from a byte slice whose length is "confirmed", omitting error check
		binary.Read(r, binary.LittleEndian, &be.blockOffset)
		binary.Read(r, binary.LittleEndian, &be.blockSize)
		binary.Read(r, binary.LittleEndian, &be.fileSize)
		binary.Read(r, binary.LittleEndian, &be.flags)
	}

	// Regardless of the version the extended block is only present in archives > 4 GB
	if h.extendedBlockTableOffset > 0 {
		// Reads the extended block table entries from the input.
		// We will probably not ever end up here in case of SC2Replay files.
		if _, err = in.Seek(int64(h.extendedBlockTableOffset)+headerOffset, 0); err != nil {
			return nil, ErrInvalidArchive
		}
		m.extBlockEntryHighOffsets = make([]uint16, h.blockTableEntries)
		for i := range m.extBlockEntryHighOffsets {
			err = binary.Read(r, binary.LittleEndian, &m.extBlockEntryHighOffsets[i])
		}
		if err != nil {
			return nil, ErrInvalidArchive
		}
	}

	// Count valid files in the archive
	m.blockEntryIndices = make([]int, h.blockTableEntries)
	for i := range m.blockEntryIndices {
		if (m.blockTable[i].flags & beFlagFile) != 0 {
			m.blockEntryIndices[m.filesCount] = i
			m.filesCount++
		}
	}

	return m, nil
}

// SrcFile returns the optional source file of the MPQ.
// Returns nil if the MPQ was not constructed from a file.
func (m *MPQ) SrcFile() *os.File {
	return m.file
}

// Input returns the input source of the MPQ content.
// A non-nil result is returned even if the MPQ is constructed from a file.
func (m *MPQ) Input() io.ReadSeeker {
	return m.input
}

// UserData returns the optional data that precedes the MPQ header.
func (m *MPQ) UserData() []byte {
	if m.userData == nil {
		return nil
	}
	return m.userData.data
}

// FilesCount returns the number of files in the archive.
func (m *MPQ) FilesCount() uint32 {
	return m.filesCount
}

// FileByName returns the content of a file specified by its name from the archive.
//
// nil slice and nil error is returned if the file cannot be found.
// ErrInvalidArchive is returned if the file exists but the storing method of the file
// is not supported/implemented or some error occurs.
//
// Implementation note: this method returns:
//
//     MPQ.FileByHash(FileNameHash(name))
//
// If you need to call this frequently, it's profitable to store the hashes returned by
// FileNameHash(), and call MPQ.FileByHash() directly passing the stored hashes.
func (m *MPQ) FileByName(name string) ([]byte, error) {
	return m.FileByHash(FileNameHash(name))
}

// FileByHash returns the content of a file specified by hashes of its name from the archive.
// The required hashes of a name can be acquired using the FileNameHash() function.
//
// nil slice and nil error is returned if the file cannot be found.
// ErrInvalidArchive is returned if the file exists but the storing method of the file
// is not supported/implemented or some error occurs.
func (m *MPQ) FileByHash(h1, h2, h3 uint32) ([]byte, error) {
	hashTableEntries := m.header.hashTableEntries
	var counter uint32

	for i := h1 & (hashTableEntries - 1); ; i++ {
		if i == hashTableEntries {
			i = 0
		}

		hashEntry := m.hashTable[i]
		if hashEntry.fileBlockIndex == 0xffffffff {
			// Indicates that the hash table entry is empty, and has always been empty. Terminates search for a given file.
			break
		}

		if hashEntry.filePathHashA != h2 || hashEntry.filePathHashB != h3 {
			continue
		}

		// FOUND!

		for j := uint32(0); j < hashEntry.fileBlockIndex; j++ {
			if m.blockTable[j].flags&beFlagFile == 0 {
				counter++
			}
		}

		// File index:
		fileIndex := hashEntry.fileBlockIndex - counter
		if fileIndex < 0 || fileIndex >= m.filesCount {
			return nil, nil
		}

		blockEntryIndex := m.blockEntryIndices[fileIndex]
		// The block containing the file
		blockEntry := m.blockTable[blockEntryIndex]

		var blockOffsetBase = int64(blockEntry.blockOffset)
		if m.extBlockEntryHighOffsets != nil {
			blockOffsetBase += int64(m.extBlockEntryHighOffsets[blockEntryIndex]) << 32
		}
		if m.userData != nil {
			blockOffsetBase += int64(m.userData.headerOffset)
		}

		var blocksCount uint32
		if blockEntry.flags&beFlagSingle != 0 {
			blocksCount = 1
		} else {
			blocksCount = (blockEntry.fileSize + m.blockSize - 1) / m.blockSize
		}
		// Create a packed block offset table
		// 1 entry for each block + 1 extra + 1 extra if FLAG_EXTRA is 1
		temp := blocksCount + 1
		if blockEntry.flags&beFlagExtra != 0 {
			temp++
		}
		packedBlockOffsets := make([]uint32, temp)

		var err error
		in := m.input

		if blockEntry.flags&beFlagCompressed != 0 && blockEntry.flags&beFlagSingle == 0 {
			// We need to load the packed block offset table, we will maintain this table for unpacked files too.
			if _, err = in.Seek(blockOffsetBase, 0); err != nil {
				return nil, ErrInvalidArchive
			}
			for k := range packedBlockOffsets {
				err = binary.Read(in, binary.LittleEndian, &packedBlockOffsets[k])
			}
			if err != nil {
				return nil, ErrInvalidArchive
			}

			// Decryption would take place here
			if blockEntry.flags&beFlagEncrypted != 0 {
				return nil, ErrInvalidArchive // Decryption of packed block offset table is not yet implemented!
			}
		} else {
			if blockEntry.flags&beFlagSingle == 0 {
				for k := uint32(0); k < blocksCount; k++ {
					packedBlockOffsets[k] = k * m.blockSize
				}
				packedBlockOffsets[blocksCount] = blockEntry.blockSize
			} else {
				packedBlockOffsets[0] = 0
				packedBlockOffsets[1] = blockEntry.blockSize
			}
		}

		content := make([]byte, blockEntry.fileSize)
		var contentIndex uint32

		var inBuffer []byte
		for k := uint32(0); k < blocksCount; k++ {
			// Unpacked size of the block
			var unpackedSize uint32
			if blockEntry.flags&beFlagSingle != 0 {
				unpackedSize = blockEntry.fileSize
			} else if k < blocksCount-1 {
				unpackedSize = m.blockSize
			} else {
				unpackedSize = blockEntry.fileSize - m.blockSize*k
			}

			// Read block
			inSize := int(packedBlockOffsets[k+1] - packedBlockOffsets[k])
			if _, err = in.Seek(blockOffsetBase+int64(packedBlockOffsets[k]), 0); err != nil {
				return nil, ErrInvalidArchive
			}

			// Reuse previous inBuffer if big enough:
			if cap(inBuffer) >= inSize {
				inBuffer = inBuffer[:inSize]
			} else {
				inBuffer = make([]byte, inSize)
			}
			if _, err = io.ReadFull(in, inBuffer); err != nil {
				return nil, ErrInvalidArchive
			}

			// Check encryption (decryption would take place here)
			if blockEntry.flags&beFlagEncrypted != 0 {
				return nil, ErrInvalidArchive // Decryption of packed data block is not yet implemented!
			}
			// Check compression
			if blockEntry.flags&beFlagCompressedMulti != 0 {
				// Decompress block
				if err = decompressMulti(content[contentIndex:contentIndex+unpackedSize], inBuffer); err != nil {
					return nil, err
				}
			} else if blockEntry.flags&beFlagPKWare != 0 { // Check implosion
				// Explode block
				return nil, ErrInvalidArchive // Explosion of data block is not yet implemented!
			} else {
				// Copy block
				copy(content[contentIndex:], inBuffer)
			}

			contentIndex += unpackedSize
		}

		return content, nil
	}

	return nil, nil
}

// Close closes the MPQ and its resources.
func (m *MPQ) Close() error {
	if m.file != nil {
		return m.file.Close()
	}
	return nil
}
