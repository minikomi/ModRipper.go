package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

const (
	chunksize  = 1024
	zerostring = string(0x00)
	//wav file header
	channelCode     = "0100"
	chunkDataSize   = "10000000"
	compressionCode = "0100"
	sampleRate      = "7A460000"
	averageBPS      = "F48C0000"
	blockAlign      = "0200"
	bitsPerSample   = "1000"
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

	wav.FormatHeader.WriteString("fmt " +
		chunkDataSize +
		compressionCode +
		channelCode +
		sampleRate +
		averageBPS +
		blockAlign +
		bitsPerSample)
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

func BigEndianBytesToInt(b []byte) uint16 {
	var ret uint16
	buf := bytes.NewBuffer(b)
	err := binary.Read(buf, binary.BigEndian, &ret)
	if err != nil {
		return 0
	}
	return ret
}

func LittleEndianBytesToInt(b []byte) uint16 {
	var ret uint16
	buf := bytes.NewBuffer(b)
	err := binary.Read(buf, binary.LittleEndian, &ret)
	if err != nil {
		return 0
	}
	return ret
}

func ProtrackerParse(rawData *bytes.Buffer) (samples []Sample) {
	modTitle := string(bytes.Trim(rawData.Next(20), zerostring))
	for i := 0; i < 31; i++ {
		sampleTitle := modTitle + " - " + string(bytes.Trim(rawData.Next(22), zerostring))
		sampleLength := BigEndianBytesToInt(rawData.Next(2)) * 2
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

func FastTrackerParse(rawData *bytes.Buffer) (samples []Sample) {
	header := string(rawData.Next(17))
	fmt.Println(header)
	if header != "Extended Module: " {
		return nil
	}
	xmTitle := string(bytes.Trim(rawData.Next(20), " "))
	fmt.Println(xmTitle)

	_ = rawData.Next(1 + //ID=01Ah
		20 + //Tracker name
		2 + //Tracker revision number, hi-byte is major version
		4 + //Header size
		2 + //Song length in patterns
		2 + //Restart position
		2) //Number of channels

	p := rawData.Next(2)
	fmt.Println(p)
	q := rawData.Next(2)
	fmt.Println(q)
	numberOfPatterns := LittleEndianBytesToInt(p)
	numberOfInstruments := LittleEndianBytesToInt(q)

	_ = rawData.Next(2 + //Flags
		2 + //Default tempo
		2 + //Default BPM
		256) // Pattern order table

	for i := uint16(0); i < numberOfPatterns; i++ {
		_ = rawData.Next(7)
		patternDataLength := LittleEndianBytesToInt(rawData.Next(2))
		_ = rawData.Next(int(patternDataLength))
	}
	_ = rawData.Next(4)
	fmt.Println(xmTitle, numberOfPatterns, numberOfInstruments, string(rawData.Next(20)))
	return nil
}

func main() {
	flag.Parse()
	if len(flag.Args()) == 0 {
		panic("Need some files.. ")
	}
	for _, f := range flag.Args() {
		rawData := bufferFromFile(f)
		dotSlice := strings.Split(f, ".")
		extensionString := strings.ToLower(dotSlice[len(dotSlice)-1])
		fmt.Println(extensionString)
		var samples []Sample
		if extensionString == "mod" {
			samples = ProtrackerParse(rawData)
		} else if extensionString == "xm" {
			fmt.Println("xm OK")
			samples = FastTrackerParse(rawData)
		} else {
			fmt.Println("Unsupported extension:", extensionString)
		}
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
