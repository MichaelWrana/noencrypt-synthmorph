package noencryptsynthmorph

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

/*
QUEUE FOR MANAGING THE SIZE AND TIME OF EACH PACKET AS DETERMINED BY AI
*/

// IntQueue is a thread-safe queue for positive integers
type IntQueue struct {
	items []int
	lock  sync.Mutex
}

// Enqueue adds a positive integer to the queue
func (q *IntQueue) Enqueue(val int) error {
	if val <= 0 {
		return errors.New("only positive integers allowed")
	}
	q.lock.Lock()
	defer q.lock.Unlock()
	q.items = append(q.items, val)
	return nil
}

// Dequeue removes and returns the front element of the queue
func (q *IntQueue) Dequeue() (int, error) {
	q.lock.Lock()
	defer q.lock.Unlock()
	if len(q.items) == 0 {
		return 0, errors.New("queue is empty")
	}
	val := q.items[0]
	q.items = q.items[1:]
	return val, nil
}

// Peek returns the front element without removing it
func (q *IntQueue) Peek() (int, error) {
	q.lock.Lock()
	defer q.lock.Unlock()
	if len(q.items) == 0 {
		return 0, errors.New("queue is empty")
	}
	return q.items[0], nil
}

// Size returns the number of elements in the queue
func (q *IntQueue) Size() int {
	q.lock.Lock()
	defer q.lock.Unlock()
	return len(q.items)
}

// IsEmpty returns true if the queue has no elements
func (q *IntQueue) IsEmpty() bool {
	return q.Size() == 0
}

/*
STRUCT FOR MANAGING KEY EXCHANGE STATE INFORMATION
*/

// SynthmorphState holds RTP and queue state
type SynthmorphState struct {
	SSRC        uint32
	SizeQueue   IntQueue
	TimingQueue IntQueue
}

// Constructor
func NewSynthmorphState() SynthmorphState {
	return SynthmorphState{
		SSRC:        0x12345678,
		SizeQueue:   IntQueue{items: make([]int, 0)},
		TimingQueue: IntQueue{items: make([]int, 0)},
	}
}

/*
UPDATE THE QUEUE OF TIMINGS AND SIZES
*/
func (s *SynthmorphState) UpdateQueue(refresh time.Duration, threshold int, count int) {
	lastTiming := 0

	for {
		time.Sleep(refresh)

		// Fill SizeQueue if needed
		if s.SizeQueue.Size() < threshold {
			for i := 0; i < count; i++ {
				sizeVal := rand.Intn(50) + 1 // Positive int [1,50]
				_ = s.SizeQueue.Enqueue(sizeVal)
			}
		}

		// Fill TimingQueue if needed
		if s.TimingQueue.Size() < threshold {
			for i := 0; i < count; i++ {
				increment := rand.Intn(5) + 1 // [1,5]
				lastTiming += increment
				_ = s.TimingQueue.Enqueue(lastTiming)
			}
		}
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

/*
MAIN SENDER/RECEIVER TOOLS
*/
func (s *SynthmorphState) QueueDeterminedSender(videoTrack *webrtc.TrackLocalStaticRTP) {
	seq := uint16(0)
	timestamp := uint32(0)
	prevTiming := 0
	const clockRate = 90000 // 90 kHz RTP clock for video
	lastSendTime := time.Now()

	for {
		// Dequeue timing value
		currTiming, _ := s.TimingQueue.Dequeue()

		// Compute intended inter-packet delay
		var delayMs int
		if prevTiming == 0 {
			delayMs = 0 // no delay before first packet
		} else {
			delayMs = currTiming - prevTiming
		}
		prevTiming = currTiming

		// Compute actual elapsed time since last packet
		now := time.Now()
		elapsed := now.Sub(lastSendTime)
		elapsedMs := int(elapsed.Milliseconds())

		sleepMs := delayMs - elapsedMs
		if sleepMs > 0 {
			time.Sleep(time.Duration(sleepMs) * time.Millisecond)
		}
		lastSendTime = time.Now()

		// Update RTP timestamp based on intended delay
		timestamp += uint32(delayMs * (clockRate / 1000))

		// Dequeue size
		size, _ := s.SizeQueue.Dequeue()

		// Assemble payload efficiently
		message := bytes.Repeat([]byte("A"), size)

		fmt.Printf("Sending packet: size=%d bytes, delay=%dms, ts=%d\n", size, delayMs, timestamp)

		pkt := &rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				PayloadType:    96,
				SequenceNumber: seq,
				Timestamp:      timestamp,
				SSRC:           0x11223344,
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
