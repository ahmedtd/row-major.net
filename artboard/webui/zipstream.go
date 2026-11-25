package webui

import (
	"bufio"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
)

// This file implements a forward-only zip reader.  archive/zip needs random
// access to the central directory at the end of the archive, but uploads are
// streamed and the server has no room to spool them.  Instead, entries are
// read in order using their local file headers.
//
// The awkward case is an entry that is stored (uncompressed) and whose size is
// only recorded in a data descriptor after its data.  Its end is found by
// looking for a descriptor whose checksum and size match the data so far;
// this fails only if the descriptor lacks its (optional) signature, which
// archivers essentially never do.

const (
	zipLocalHeaderSig    = 0x04034b50
	zipCentralHeaderSig  = 0x02014b50
	zipEndSig            = 0x06054b50
	zip64EndSig          = 0x06064b50
	zipDataDescriptorSig = 0x08074b50

	zipFlagEncrypted      = 0x1
	zipFlagDataDescriptor = 0x8

	zipMethodStore   = 0
	zipMethodDeflate = 8

	zip64ExtraID = 0x0001
)

var (
	errZipFormat   = errors.New("The archive is not a valid zip file")
	errZipTooLarge = errors.New("The archive's contents are too large")
	errZipChecksum = errors.New("The archive is corrupt (checksum mismatch)")

	// errZipUnsupported is for entries that are encrypted or use a
	// compression method other than deflate.  They are skipped if possible.
	errZipUnsupported = errors.New("The archive contains an entry in an unsupported format")
)

// zipStream reads the entries of a zip archive in order.
type zipStream struct {
	r *bufio.Reader

	// remaining bounds the total decompressed bytes read from all entries,
	// as a defense against zip bombs.
	remaining int64

	cur  *zipEntry
	done bool
}

// newZipStream reads a zip archive from r.  At most maxBytes of decompressed
// content will be read across all entries.
func newZipStream(r io.Reader, maxBytes int64) *zipStream {
	// bufio.Reader implements io.ByteReader, so flate reads exactly to the
	// end of each compressed stream and no further.
	return &zipStream{r: bufio.NewReaderSize(r, 64<<10), remaining: maxBytes}
}

// zipEntry is a single file (or directory) within the archive.  Reading it
// yields the decompressed content; the checksum is verified on reaching the
// end.
type zipEntry struct {
	Name string

	z          *zipStream
	flags      uint16
	method     uint16
	crc        uint32
	compressed uint64
	size       uint64
	zip64      bool

	// raw is the compressed data.  It is limited to the entry when the size
	// is known up front, and is the whole remaining stream otherwise.
	raw  io.Reader
	body io.Reader

	hash hash.Hash32
	n    uint64
	err  error
}

// sizeKnown reports whether the local header records the entry's size.
func (e *zipEntry) sizeKnown() bool {
	return e.flags&zipFlagDataDescriptor == 0
}

// supported reports whether the entry's content can be read.
func (e *zipEntry) supported() bool {
	return e.flags&zipFlagEncrypted == 0 && (e.method == zipMethodStore || e.method == zipMethodDeflate)
}

func (e *zipEntry) Read(p []byte) (int, error) {
	if e.err != nil {
		return 0, e.err
	}
	if int64(len(p)) > e.z.remaining+1 {
		p = p[:e.z.remaining+1]
	}
	n, err := e.body.Read(p)
	e.hash.Write(p[:n])
	e.n += uint64(n)
	e.z.remaining -= int64(n)
	if e.z.remaining < 0 {
		err = errZipTooLarge
	} else if err == io.EOF {
		err = e.finish()
		if err == nil {
			err = io.EOF
		}
	} else if err != nil && !errors.Is(err, errZipTooLarge) {
		err = fmt.Errorf("while reading %q from archive: %w", e.Name, err)
	}
	e.err = err
	return n, err
}

// finish reads the data descriptor, if any, and checks the content against
// the recorded checksum and size.
func (e *zipEntry) finish() error {
	if !e.sizeKnown() {
		var sig [4]byte
		if _, err := io.ReadFull(e.z.r, sig[:]); err != nil {
			return errZipFormat
		}
		sizeLen := 4
		if e.zip64 {
			sizeLen = 8
		}
		// The descriptor's signature is optional; without it, what was
		// read is the checksum.
		rest := make([]byte, 4+2*sizeLen)
		toRead := rest
		if binary.LittleEndian.Uint32(sig[:]) != zipDataDescriptorSig {
			copy(rest, sig[:])
			toRead = rest[4:]
		}
		if _, err := io.ReadFull(e.z.r, toRead); err != nil {
			return errZipFormat
		}
		e.crc = binary.LittleEndian.Uint32(rest[0:4])
		if e.zip64 {
			e.size = binary.LittleEndian.Uint64(rest[12:20])
		} else {
			e.size = uint64(binary.LittleEndian.Uint32(rest[8:12]))
		}
	}
	if e.hash.Sum32() != e.crc || e.n != e.size {
		return errZipChecksum
	}
	return nil
}

// Next advances to the next entry, skipping whatever remains of the current
// one.  It returns io.EOF after the last entry.
func (z *zipStream) Next() (*zipEntry, error) {
	if z.done {
		return nil, io.EOF
	}
	if err := z.skipCurrent(); err != nil {
		return nil, err
	}

	var sig [4]byte
	if _, err := io.ReadFull(z.r, sig[:]); err != nil {
		return nil, errZipFormat
	}
	switch binary.LittleEndian.Uint32(sig[:]) {
	case zipLocalHeaderSig:
	case zipCentralHeaderSig, zipEndSig, zip64EndSig:
		// The entries are over; what follows is the central directory,
		// which just repeats what the local headers said.
		z.done = true
		return nil, io.EOF
	default:
		return nil, errZipFormat
	}

	var hdr [26]byte
	if _, err := io.ReadFull(z.r, hdr[:]); err != nil {
		return nil, errZipFormat
	}
	e := &zipEntry{
		z:          z,
		flags:      binary.LittleEndian.Uint16(hdr[2:4]),
		method:     binary.LittleEndian.Uint16(hdr[4:6]),
		crc:        binary.LittleEndian.Uint32(hdr[10:14]),
		compressed: uint64(binary.LittleEndian.Uint32(hdr[14:18])),
		size:       uint64(binary.LittleEndian.Uint32(hdr[18:22])),
		hash:       crc32.NewIEEE(),
	}
	nameLen := binary.LittleEndian.Uint16(hdr[22:24])
	extraLen := binary.LittleEndian.Uint16(hdr[24:26])

	name := make([]byte, nameLen)
	if _, err := io.ReadFull(z.r, name); err != nil {
		return nil, errZipFormat
	}
	e.Name = string(name)

	extra := make([]byte, extraLen)
	if _, err := io.ReadFull(z.r, extra); err != nil {
		return nil, errZipFormat
	}
	if err := e.parseExtra(extra); err != nil {
		return nil, err
	}

	if e.sizeKnown() {
		e.raw = io.LimitReader(z.r, int64(min(e.compressed, 1<<62)))
	} else {
		if !e.supported() {
			// There's no way to find the end of the entry to skip it.
			return nil, fmt.Errorf("%w: %q", errZipUnsupported, e.Name)
		}
		e.raw = z.r
	}

	switch {
	case !e.supported():
		e.err = fmt.Errorf("%w: %q", errZipUnsupported, e.Name)
	case e.method == zipMethodStore && !e.sizeKnown():
		e.body = &storedDescriptorReader{e: e}
	case e.method == zipMethodStore:
		e.body = e.raw
	case e.method == zipMethodDeflate:
		e.body = flate.NewReader(e.raw)
	}

	z.cur = e
	return e, nil
}

// parseExtra picks the 64-bit sizes out of the local header's extra fields.
func (e *zipEntry) parseExtra(extra []byte) error {
	for len(extra) >= 4 {
		id := binary.LittleEndian.Uint16(extra[0:2])
		n := int(binary.LittleEndian.Uint16(extra[2:4]))
		extra = extra[4:]
		if n > len(extra) {
			return errZipFormat
		}
		field := extra[:n]
		extra = extra[n:]
		if id != zip64ExtraID {
			continue
		}
		e.zip64 = true
		// The local header's zip64 field holds both sizes, but only if
		// they didn't fit in the header proper.
		if e.size == 0xffffffff {
			if len(field) < 8 {
				return errZipFormat
			}
			e.size = binary.LittleEndian.Uint64(field[0:8])
			field = field[8:]
		}
		if e.compressed == 0xffffffff {
			if len(field) < 8 {
				return errZipFormat
			}
			e.compressed = binary.LittleEndian.Uint64(field[0:8])
		}
	}
	return nil
}

// skipCurrent moves the stream past the rest of the current entry.
func (z *zipStream) skipCurrent() error {
	e := z.cur
	if e == nil {
		return nil
	}
	z.cur = nil

	if e.supported() {
		if _, err := io.Copy(io.Discard, e); err != nil {
			return err
		}
	} else if e.err == nil || !e.sizeKnown() {
		// Unreachable: unsupported entries are given an error, and those of
		// unknown size are rejected by Next.
		return errZipFormat
	}
	if e.sizeKnown() {
		// Discard any compressed bytes the decompressor didn't consume (or
		// all of them, if the entry couldn't be read).
		if _, err := io.Copy(io.Discard, e.raw); err != nil {
			return fmt.Errorf("while skipping %q in archive: %w", e.Name, err)
		}
	}
	return nil
}

// storedDescriptorReader reads a stored entry whose size is unknown, stopping
// just before the data descriptor that follows it.
type storedDescriptorReader struct {
	e *zipEntry
}

func (s *storedDescriptorReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	r := s.e.z.r
	descLen := 16
	if s.e.zip64 {
		descLen = 24
	}
	if _, err := r.Peek(descLen); err != nil {
		// The stream ended without a descriptor.
		return 0, errZipFormat
	}
	b, _ := r.Peek(r.Buffered())

	// Every position before end could be the start of a descriptor that is
	// fully buffered, so is checked; later positions aren't checked yet.
	end := len(b) - descLen + 1
	sig := []byte{'P', 'K', 7, 8}
	for i := 0; i < end; {
		j := bytes.Index(b[i:end+3], sig)
		if j < 0 {
			break
		}
		i += j
		if s.isDescriptor(b[:i], b[i:i+descLen]) {
			end = i
			break
		}
		i++
	}
	if end == 0 {
		return 0, io.EOF
	}
	n := copy(p, b[:end])
	r.Discard(n)
	return n, nil
}

// isDescriptor reports whether desc is a data descriptor for the entry's
// content so far followed by data.
func (s *storedDescriptorReader) isDescriptor(data, desc []byte) bool {
	crc := crc32.Update(s.e.hash.Sum32(), crc32.IEEETable, data)
	size := s.e.n + uint64(len(data))
	if binary.LittleEndian.Uint32(desc[4:8]) != crc {
		return false
	}
	if s.e.zip64 {
		return binary.LittleEndian.Uint64(desc[8:16]) == size && binary.LittleEndian.Uint64(desc[16:24]) == size
	}
	return uint64(binary.LittleEndian.Uint32(desc[8:12])) == size && uint64(binary.LittleEndian.Uint32(desc[12:16])) == size
}
