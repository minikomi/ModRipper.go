package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
)

const (
	chunksize  = 1024
	zerostring = string(byte(00))
)

var (
	channelCode, _     = hex.DecodeString("0100")
	chunkDataSize, _   = hex.DecodeString("10000000")
	compressionCode, _ = hex.DecodeString("0100")
	sampleRate, _      = hex.DecodeString("7A460000")
	averageBPS, _      = hex.DecodeString("F48C0000")
	blockAlign, _      = hex.DecodeString("0200")
	bitsPerSample, _   = hex.DecodeString("1000")
)

func bufferFromFile(name string) (buffer *bytes.Buffer) {
	data, err := os.Open(name)
	if err != nil {
		log.Fatal(err)
	}
	defer data.Close()

	reader := bufio.NewReader(data)
	buffer = bytes.NewBuffer(make([]byte, 0))
	part := make([]byte, chunksize)
	for {
		count, err := reader.Read(part)
		if err != nil {
			break
		}
		buffer.Write(part[:count])
	}
	if err == io.EOF {
		err = nil
	}
	return
}

type Sample struct {
	Title  string
	Length int
	Data   []byte
}

type WavFile struct {
	Filename     string
	Header       *bytes.Buffer
	FormatHeader *bytes.Buffer
	Data         *bytes.Buffer
}

func NewWavFile() (wav *WavFile) {
	wav = &WavFile{
		Header:       bytes.NewBuffer(make([]byte, 0)),
		FormatHeader: bytes.NewBuffer(make([]byte, 0)),
		Data:         bytes.NewBuffer(make([]byte, 0)),
	}
	//prepare header
	wav.Header.WriteString("RIFF")
	//prepare format header

	wav.FormatHeader.WriteString("fmt ")
	wav.FormatHeader.Write(chunkDataSize)
	wav.FormatHeader.Write(compressionCode)
	wav.FormatHeader.Write(channelCode)
	wav.FormatHeader.Write(sampleRate)
	wav.FormatHeader.Write(averageBPS)
	wav.FormatHeader.Write(blockAlign)
	wav.FormatHeader.Write(bitsPerSample)
	//prepare data header
	wav.Data.WriteString("data")
	return
}

func (w *WavFile) Construct(s Sample) {
	w.Filename = s.Title
	w.Data.Write(bytesFromInt(len(s.Data)))
	for _, byt := range s.Data {
		w.Data.WriteByte(byt)
	}
	w.Header.Write(bytesFromInt(len(s.Data) + w.FormatHeader.Len() + 8))
	w.Header.WriteString("WAVE")
}

func (w *WavFile) Dump() (err error) {
	dump, err := os.OpenFile(w.Filename+".wav", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return
	}
	defer dump.Close()
	_, err = dump.Write(w.Header.Bytes())
	if err != nil {
		return
	}
	_, err = dump.Write(w.FormatHeader.Bytes())
	if err != nil {
		return
	}
	_, err = dump.Write(w.Data.Bytes())
	if err != nil {
		return
	}
	return
}

func bytesFromInt(n int) (data []byte) {
	data = make([]byte, 4)
	binary.LittleEndian.PutUint32(data[:], uint32(n))
	return
}

func biggest(b []byte) (x byte) {
	for _, y := range b {
		if y > x {
			x = y
		}
	}
	return x
}

func ProtrackerParse(rawData *bytes.Buffer) (samples []Sample) {

	modTitle := string(bytes.Trim(rawData.Next(20), zerostring))
	for i := 0; i < 31; i++ {
		sampleTitle := modTitle + " - " + string(bytes.Trim(rawData.Next(22), zerostring))
		sampleLengthBytes := rawData.Next(2)
		var sampleLength uint16
		buf := bytes.NewBuffer(sampleLengthBytes)
		err := binary.Read(buf, binary.BigEndian, &sampleLength)
		sampleLength = sampleLength * 2
		if err != nil {
			break
		}
		if sampleLength >= uint16(2) &&
			len(sampleTitle) > 0 {
			samples = append(samples, Sample{
				Title:  sampleTitle,
				Length: int(sampleLength),
			})
		}
		//discard finetune (1 byte), volume (1 byte), repeat info (4 bytes)
		_ = rawData.Next(6)
	}

	songLength := rawData.Next(2)[0] //discard unused 127 byte.
	patternOrder := rawData.Next(133)[:songLength]
	// discard pattern data
	for i := 0; i < int(biggest(patternOrder))+1; i++ { // patterns start at 00, so add 1
		_ = rawData.Next(1024)
	}

	for i, s := range samples {
		samples[i].Data = rawData.Next(s.Length)
	}

	fmt.Println("Title:", modTitle, "Samples:", len(samples))
	return
}

func main() {
	flag.Parse()
	if len(flag.Args()) == 0 {
		panic("Need some files.. ")
	}
	for _, f := range flag.Args() {
		rawData := bufferFromFile(f)
		samples := ProtrackerParse(rawData)
		for _, s := range samples {
			wav := NewWavFile()
			wav.Construct(s)
			err := wav.Dump()
			if err != nil {
				fmt.Println("Error:", s.Title, err)
			} else {
				fmt.Println("Wrote wav:", wav.Filename)
			}
		}
	}
}
