package spectralimage

import (
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"row-major/harpoon/spectralimage/headerproto"

	"google.golang.org/protobuf/proto"
)

type SpectralImage struct {
	RowSize, ColSize, WavelengthSize int
	WavelengthMin, WavelengthMax     float32
	PowerDensitySums                 []float32
	PowerDensityCounts               []float32
}

type SpectralImageSample struct {
	WavelengthLo, WavelengthHi         float32
	PowerDensitySum, PowerDensityCount float32
}

func (s *SpectralImage) Resize(rowSize, colSize, wavelengthSize int) {
	s.RowSize = rowSize
	s.ColSize = colSize
	s.WavelengthSize = wavelengthSize

	s.PowerDensitySums = make([]float32, rowSize*colSize*wavelengthSize)
	s.PowerDensityCounts = make([]float32, rowSize*colSize*wavelengthSize)
}

func (s *SpectralImage) WavelengthBin(i int) (float32, float32) {
	binWidth := (s.WavelengthMax - s.WavelengthMin) / float32(s.WavelengthSize)
	lo := s.WavelengthMin + float32(i)*binWidth
	if i == s.WavelengthSize-1 {
		return lo, s.WavelengthMax
	}
	return lo, lo + binWidth
}

func (s *SpectralImage) RecordSample(r, c, w int, powerDensity float32) {
	idx := r*s.ColSize*s.WavelengthSize + c*s.WavelengthSize + w
	s.PowerDensitySums[idx] += powerDensity
	s.PowerDensityCounts[idx] += 1
}

func (s *SpectralImage) ReadSample(r, c, w int) SpectralImageSample {
	idx := r*s.ColSize*s.WavelengthSize + c*s.WavelengthSize + w
	binLo, binHi := s.WavelengthBin(w)
	return SpectralImageSample{
		WavelengthLo:      binLo,
		WavelengthHi:      binHi,
		PowerDensitySum:   s.PowerDensitySums[idx],
		PowerDensityCount: s.PowerDensityCounts[idx],
	}
}

func (s *SpectralImage) Cut(rowSrc, rowLim, colSrc, colLim int) *SpectralImage {
	dst := &SpectralImage{
		WavelengthMin: s.WavelengthMin,
		WavelengthMax: s.WavelengthMax,
	}
	dst.Resize(rowLim-rowSrc, colLim-colSrc, s.WavelengthSize)

	dstIndex := 0
	for r := rowSrc; r < rowLim; r++ {
		for c := colSrc; c < colLim; c++ {
			for w := 0; w < s.WavelengthSize; w++ {
				srcIndex := r*s.ColSize*s.WavelengthSize + c*s.WavelengthSize + w

				dst.PowerDensitySums[dstIndex] = s.PowerDensitySums[srcIndex]
				dst.PowerDensityCounts[dstIndex] = s.PowerDensityCounts[srcIndex]

				dstIndex++
			}
		}
	}

	return dst
}

func (s *SpectralImage) Paste(src *SpectralImage, rowSrc, colSrc int) {
	rowLim := rowSrc + src.RowSize
	colLim := colSrc + src.ColSize

	for r := rowSrc; r < rowLim; r++ {
		for c := colSrc; c < colLim; c++ {
			for w := 0; w < s.WavelengthSize; w++ {
				dstIndex := r*s.ColSize*s.WavelengthSize + c*s.WavelengthSize + w
				srcIndex := (r-rowSrc)*src.ColSize*s.WavelengthSize + (c-colSrc)*src.WavelengthSize + w

				s.PowerDensitySums[dstIndex] = src.PowerDensitySums[srcIndex]
				s.PowerDensityCounts[dstIndex] = src.PowerDensityCounts[srcIndex]
			}
		}
	}
}

func ReadSpectralImage(in io.Reader) (*SpectralImage, error) {
	// Read header length.
	var headerLength uint64
	if err := binary.Read(in, binary.LittleEndian, &headerLength); err != nil {
		return nil, fmt.Errorf("while reading header length: %w", err)
	}

	headerBytes := make([]byte, int(headerLength))
	if _, err := in.Read(headerBytes); err != nil {
		return nil, fmt.Errorf("while reading header bytes: %w", err)
	}

	hdr := &headerproto.SpectralImageHeader{}
	if err := proto.Unmarshal(headerBytes, hdr); err != nil {
		return nil, fmt.Errorf("while unmarshaling header: %w", err)
	}

	if hdr.GetDataLayoutVersion() != 1 {
		return nil, fmt.Errorf("bad data layout version: %v", hdr.GetDataLayoutVersion())
	}

	im := &SpectralImage{}
	im.WavelengthMin = hdr.GetWavelengthMin()
	im.WavelengthMax = hdr.GetWavelengthMax()

	im.Resize(int(hdr.GetRowSize()), int(hdr.GetColSize()), int(hdr.GetWavelengthSize()))

	zipReader, err := zlib.NewReader(in)
	if err != nil {
		return nil, fmt.Errorf("while opening zip reader: %w", err)
	}
	defer zipReader.Close()

	if err := binary.Read(zipReader, binary.LittleEndian, &im.PowerDensitySums); err != nil {
		return nil, fmt.Errorf("while reading power density sums: %w", err)
	}

	if err := binary.Read(zipReader, binary.LittleEndian, &im.PowerDensityCounts); err != nil {
		return nil, fmt.Errorf("while reading power density counts: %w", err)
	}

	return im, nil
}

func ReadSpectralImageFromFile(name string) (*SpectralImage, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("while opening file: %w", err)
	}
	defer f.Close()

	return ReadSpectralImage(f)
}

func WriteSpectralImage(im *SpectralImage, w io.Writer) error {
	hdr := &headerproto.SpectralImageHeader{
		RowSize:           uint32(im.RowSize),
		ColSize:           uint32(im.ColSize),
		WavelengthSize:    uint32(im.WavelengthSize),
		WavelengthMin:     im.WavelengthMin,
		WavelengthMax:     im.WavelengthMax,
		DataLayoutVersion: uint32(1),
	}

	hdrBytes, err := proto.Marshal(hdr)
	if err != nil {
		return fmt.Errorf("while marshaling header: %w", err)
	}

	headerLengthBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(headerLengthBytes, uint64(len(hdrBytes)))
	if _, err := w.Write(headerLengthBytes); err != nil {
		return fmt.Errorf("while writing header length: %w", err)
	}

	if _, err := w.Write(hdrBytes); err != nil {
		return fmt.Errorf("while writing header: %w", err)
	}

	zipWriter := zlib.NewWriter(w)

	if err := binary.Write(zipWriter, binary.LittleEndian, im.PowerDensitySums); err != nil {
		return fmt.Errorf("while writing power density sums: %w", err)
	}

	if err := binary.Write(zipWriter, binary.LittleEndian, im.PowerDensityCounts); err != nil {
		return fmt.Errorf("while writing power density counts: %w", err)
	}

	if err := zipWriter.Flush(); err != nil {
		return fmt.Errorf("while flushing zip writer: %w", err)
	}

	if err := zipWriter.Close(); err != nil {
		return fmt.Errorf("while closing zip writer: %w", err)
	}

	return nil
}
