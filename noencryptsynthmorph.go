package noencryptsynthmorph

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

/*
STRUCT FOR MANAGING KEY EXCHANGE STATE INFORMATION
*/

type SynthmorphState struct {
	//cryptographic state information
	Lock sync.Mutex
	//RTP connection state information
	SSRC uint32
}

func NewSynthmorphState() SynthmorphState {
	state := SynthmorphState{}
	state.SSRC = 0x12345678
	state.Lock = sync.Mutex{}
	return state
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

/*
MAIN SENDER/RECEIVER TOOLS
*/

func (s *SynthmorphState) SendData(videoTrack *webrtc.TrackLocalStaticRTP, header byte, payload []byte) {
	seq := uint16(1)
	timestamp := uint32(0)

	message := append([]byte{header}, payload...)

	pkt := &rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    96, // Dynamic payload type (e.g., for VP8)
			SequenceNumber: seq,
			Timestamp:      timestamp,
		},
		// Set payload to "Hello World!"
		Payload: message,
	}
	if err := videoTrack.WriteRTP(pkt); err != nil {
		panic(err)
	}

	fmt.Printf("##### Sent Pkt, seqnum=%v##### \n", seq)
}

// interval is in seconds
func (s *SynthmorphState) SynthmorphPeriodicSender(videoTrack *webrtc.TrackLocalStaticRTP, interval int32) {
	seq := uint16(0)
	timestamp := uint32(0)

	message := []byte("Hello World!")

	// Set ticker interval to 5 seconds
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {

		fmt.Printf("===== Sending Msg: %s =====\n", message)

		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96, // Dynamic payload type (e.g., for VP8)
				SequenceNumber: seq,
				Timestamp:      timestamp,
				SSRC:           0x11223344, // Example SSRC; typically randomized
			},
			// Set payload to "Hello World!"
			Payload: message,
		}

		if err := videoTrack.WriteRTP(pkt); err != nil {
			panic(err)
		}

		fmt.Printf("##### Sent Pkt, seqnum=%v##### \n", seq)

		// Increment header fields for the next packet.
		seq++
		timestamp += 450000
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
