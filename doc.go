/*

Package mpq is a decoder/parser of Blizzard's MPQ archive file format.

This is not a full implementation, primarily intended to parse StarCraft II replay files (*.SC2Replay).

Information sources:

- The_MoPaQ_Archive_Format: http://wiki.devklog.net/index.php?title=The_MoPaQ_Archive_Format

- MPQ on wikipedia: http://en.wikipedia.org/wiki/MPQ

- Zezula MPQ description: http://www.zezula.net/mpq.html

- Stormlib: https://github.com/ladislav-zezula/StormLib

- Libmpq project: https://github.com/ge0rg/libmpq (old: https://libmpq.org/)

- MPQ parser of the Scelight project: https://github.com/icza/scelight/tree/master/src-app-libs/hu/belicza/andras/mpq

Format of the "(attributes)" meta attributes file:
href="https://github.com/stormlib/StormLib/blob/3a926f0228c68d7d91cf3946624d7859976440ec/src/SFileAttributes.cpp

    int version: Version of the (attributes) file. Must be 100 (0x64)
    int flags: flags telling what is contained in the "(attributes)"
        MPQ_ATTRIBUTE_CRC32         0x00000001  The "(attributes)" contains CRC32 for each file
        MPQ_ATTRIBUTE_FILETIME      0x00000002  The "(attributes)" contains file time for each file
        MPQ_ATTRIBUTE_MD5           0x00000004  The "(attributes)" contains MD5 for each file
        MPQ_ATTRIBUTE_PATCH_BIT     0x00000008  The "(attributes)" contains a patch bit for each file
        MPQ_ATTRIBUTE_ALL           0x0000000F  Summary mask

If has CRC32: int * BlockTableSize
If has FILETIME: long * BlockTableSize
If has MD5: MD5SIZE * BlockTableSize
If has PATCH_BIT: enough bytes to hold BlockTableSize bits

*/
package mpq
