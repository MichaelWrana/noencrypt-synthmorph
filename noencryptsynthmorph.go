package noencryptsynthmorph

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

/*
STRUCT FOR MANAGING KEY EXCHANGE STATE INFORMATION
*/

// SynthmorphState holds RTP and queue state
type SynthmorphState struct {
	SSRC      uint32
	SizeQueue chan int
	TimeQueue chan int
	csvFile   *os.File
	csvReader *csv.Reader
}

// Constructor
func NewSynthmorphState(numPktsQueue int) SynthmorphState {
	return SynthmorphState{
		SSRC:      0x12345678,
		SizeQueue: make(chan int, numPktsQueue),
		TimeQueue: make(chan int, numPktsQueue),
	}
}

/*
LOCAL HELPER FUNCTIONS
*/

func printRTPPacket(packet *rtp.Packet) {
	// Print header details.
	fmt.Printf("RTP Header:\n")
	fmt.Printf("  Version: %d\n", packet.Version)
	fmt.Printf("  Padding: %v\n", packet.Padding)
	fmt.Printf("  Extension: %v\n", packet.Extension)
	fmt.Printf("  Marker: %v\n", packet.Marker)
	fmt.Printf("  PayloadType: %d\n", packet.PayloadType)
	fmt.Printf("  SequenceNumber: %d\n", packet.SequenceNumber)
	fmt.Printf("  Timestamp: %d\n", packet.Timestamp)
	fmt.Printf("  SSRC: %d\n", packet.SSRC)

	payloadStr := string(packet.Payload)
	fmt.Printf("Payload (string): %s\n", payloadStr)
	fmt.Printf("Payload (hex): %x\n", packet.Payload)
}

func (s *SynthmorphState) LoadCSV(path string) {
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	_, _ = reader.Read() // skip header

	for {
		record, err := reader.Read()
		if err == io.EOF {
			close(s.SizeQueue)
			close(s.TimeQueue)
			print("Reached end of CSV")
			return
		} else if err != nil {
			panic(err)
		}

		timestampFile, _ := strconv.ParseFloat(record[0], 64)
		sizeFile, _ := strconv.ParseFloat(record[1], 64)

		s.SizeQueue <- int(math.Floor(sizeFile) - 28)
		s.TimeQueue <- int(timestampFile * 1000)

	}

}

/*
MAIN SENDER/RECEIVER TOOLS
*/

func (s *SynthmorphState) QueueDeterminedSender(videoTrack *webrtc.TrackLocalStaticRTP) {
	seq := uint16(0)
	timestamp := uint32(0)
	const clockRate = 90000 // 90 kHz RTP clock for video

	startTime := time.Now()
	first := true

	for {
		// Dequeue timing value
		currTiming, timeOk := <-s.TimeQueue
		// Dequeue size
		size, sizeOk := <-s.SizeQueue

		if !timeOk || !sizeOk {
			fmt.Println("CSV File has been emptied, exiting...")
			os.Exit(0)
			return
		}

		if first {
			startTime = time.Now()
			first = false
		}

		targetSendTime := startTime.Add(time.Duration(currTiming) * time.Millisecond)
		timeUntilSend := time.Until(targetSendTime)

		if timeUntilSend > 0 {
			time.Sleep(timeUntilSend)
		}

		// Update RTP timestamp based on absolute timing
		timestamp = uint32(currTiming * (clockRate / 1000)) // convert ms to 90kHz units

		// Assemble payload efficiently
		message := bytes.Repeat([]byte("A"), size)

		fmt.Printf("Sending packet: size=%d bytes, delay=%dms, ts=%d\n", size, timeUntilSend, timestamp)

		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: seq,
				Timestamp:      timestamp,
			},
			Payload: message,
		}

		if err := videoTrack.WriteRTP(pkt); err != nil {
			panic(err)
		}

		fmt.Printf("Sent packet: seq=%v\n", seq)
		seq++
	}
}

// Read packets
// this one should be called concurrently
func (s *SynthmorphState) SynthmorphPacketRecv(track *webrtc.TrackRemote) {
	for {
		packet, _, err := track.ReadRTP()
		if err != nil {
			log.Printf("Error reading RTP packet: %v\n", err)
			return
		}
		// figure out what to do with the incoming packet
		fmt.Printf("##### Recv Pkt, seqnum=%v##### \n", packet.SequenceNumber)
		printRTPPacket(packet)
	}
}
