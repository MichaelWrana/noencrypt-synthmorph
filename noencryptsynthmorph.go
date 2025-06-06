package noencryptsynthmorph

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"strconv"
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
	csvFile     *os.File
	csvReader   *csv.Reader
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

func (s *SynthmorphState) InitCSV(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}

	s.csvFile = file
	s.csvReader = csv.NewReader(file)

	// skip header
	_, err = s.csvReader.Read()
	return err
}

/*
UPDATE THE QUEUE OF TIMINGS AND SIZESs
*/
func (s *SynthmorphState) UpdateQueue(refresh time.Duration, threshold int, count int) {
	for {
		// check every refresh milliseconds
		time.Sleep(refresh)

		// if the queue length is below the threshold
		if s.SizeQueue.Size() < threshold {
			enqueued := 0

			// add count new packets to the queue
			for enqueued < count {
				record, err := s.csvReader.Read()
				if err == io.EOF {
					fmt.Println("Reached end of CSV - stopping")
					return
				} else if err != nil {
					fmt.Printf("CSV read error: %v\n", err)
					continue
				}

				timestampF, err1 := strconv.ParseFloat(record[0], 64)
				sizeF, err2 := strconv.ParseFloat(record[1], 64)
				if err1 != nil || err2 != nil {
					fmt.Printf("Parse error on line: %v\n", record)
					continue
				}

				timestampMS := int(timestampF * 1000)
				size := int(math.Floor(sizeF))

				_ = s.TimingQueue.Enqueue(timestampMS)
				_ = s.SizeQueue.Enqueue(size)

				enqueued++
			}
		}
	}
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
		currTiming, timeErr := s.TimingQueue.Dequeue()
		// Dequeue size
		size, sizeErr := s.SizeQueue.Dequeue()

		if timeErr != nil || sizeErr != nil {
			fmt.Println("Received stop signal — exiting sender loop")
			os.Exit(0)
			//panic(timeErr)
			return
		}

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
