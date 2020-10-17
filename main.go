package main

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nonoo/kappanhang/log"
)

var conn *net.UDPConn
var localSID uint32
var remoteSID uint32
var sendSeq uint16
var authSendSeq uint16
var authInnerSendSeq uint16
var authID [6]byte
var randIDByteForPktSeven [1]byte
var expectedPkt7ReplySeq uint16
var reauthSent bool
var lastReauthAt time.Time

func send(d []byte) {
	_, err := conn.Write(d)
	if err != nil {
		log.Fatal(err)
	}
}

func read() ([]byte, error) {
	err := conn.SetReadDeadline(time.Now().Add(time.Second))
	if err != nil {
		log.Fatal(err)
	}

	b := make([]byte, 1500)
	n, _, err := conn.ReadFromUDP(b)
	if err != nil {
		if err, ok := err.(net.Error); ok && !err.Timeout() {
			log.Fatal(err)
		}
	}
	return b[:n], err
}

func setupCloseHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Print("disconnecting")
		sendDisconnect()
		os.Exit(0)
	}()
}

func sendPkt7(replyID []byte, seq uint16) {
	// Example request from PC:  0x15, 0x00, 0x00, 0x00, 0x07, 0x00, 0x09, 0x00, 0xbe, 0xd9, 0xf2, 0x63, 0xe4, 0x35, 0xdd, 0x72, 0x00, 0x78, 0x40, 0xf6, 0x02
	// Example reply from radio: 0x00, 0x00, 0x00, 0x00, 0x07, 0x00, 0x09, 0x00, 0xe4, 0x35, 0xdd, 0x72, 0xbe, 0xd9, 0xf2, 0x63, 0x01, 0x78, 0x40, 0xf6, 0x02
	var replyFlag byte
	if replyID == nil {
		replyID = make([]byte, 4)
		var randID [2]byte
		_, err := rand.Read(randID[:])
		if err != nil {
			log.Fatal(err)
		}
		replyID[0] = randID[0]
		replyID[1] = randID[1]
		replyID[2] = randIDByteForPktSeven[0]
		replyID[3] = 0x03
	} else {
		replyFlag = 0x01
	}

	expectedPkt7ReplySeq = sendSeq

	send([]byte{0x15, 0x00, 0x00, 0x00, 0x07, 0x00, byte(seq), byte(seq >> 8),
		byte(localSID >> 24), byte(localSID >> 16), byte(localSID >> 8), byte(localSID),
		byte(remoteSID >> 24), byte(remoteSID >> 16), byte(remoteSID >> 8), byte(remoteSID),
		replyFlag, replyID[0], replyID[1], replyID[2], replyID[3]})
}

func sendPkt3() {
	send([]byte{0x10, 0x00, 0x00, 0x00, 0x03, 0x00, byte(sendSeq), byte(sendSeq >> 8),
		byte(localSID >> 24), byte(localSID >> 16), byte(localSID >> 8), byte(localSID),
		byte(remoteSID >> 24), byte(remoteSID >> 16), byte(remoteSID >> 8), byte(remoteSID)})
}

func sendPkt6() {
	send([]byte{0x10, 0x00, 0x00, 0x00, 0x06, 0x00, 0x01, 0x00,
		byte(localSID >> 24), byte(localSID >> 16), byte(localSID >> 8), byte(localSID),
		byte(remoteSID >> 24), byte(remoteSID >> 16), byte(remoteSID >> 8), byte(remoteSID)})
}

func sendPktLogin() {
	// The reply to the login packet will contain a 6 bytes long auth ID with the first 2 bytes set to our randID.
	var randID [2]byte
	_, err := rand.Read(randID[:])
	if err != nil {
		log.Fatal(err)
	}
	send([]byte{0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00,
		byte(localSID >> 24), byte(localSID >> 16), byte(localSID >> 8), byte(localSID),
		byte(remoteSID >> 24), byte(remoteSID >> 16), byte(remoteSID >> 8), byte(remoteSID),
		0x00, 0x00, 0x00, 0x70, 0x01, 0x00, 0x00, byte(authInnerSendSeq),
		byte(authInnerSendSeq >> 8), 0x00, randID[0], randID[1], 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x2b, 0x3f, 0x55, 0x5c, 0x00, 0x00, 0x00, 0x00, // username: beer
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x2b, 0x3f, 0x55, 0x5c, 0x3f, 0x25, 0x77, 0x58, // pass: beerbeer
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x69, 0x63, 0x6f, 0x6d, 0x2d, 0x70, 0x63, 0x00, // icom-pc in plain text
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	authSendSeq++
	authInnerSendSeq++
}

func sendPktReauth() {
	var magic byte

	if reauthSent {
		magic = 0x05
	} else {
		magic = 0x02
	}

	// Example request from PC:  0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0d, 0x00,
	//                           0xbb, 0x41, 0x3f, 0x2b, 0xe6, 0xb2, 0x7b, 0x7b,
	//                           0x00, 0x00, 0x00, 0x30, 0x01, 0x05, 0x00, 0x02,
	//                           0x00, 0x00, 0x5d, 0x37, 0x12, 0x82, 0x3b, 0xde,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
	// Example reply from radio: 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x0e, 0x00,
	//                           0xe6, 0xb2, 0x7b, 0x7b, 0xbb, 0x41, 0x3f, 0x2b,
	//                           0x00, 0x00, 0x00, 0x30, 0x02, 0x05, 0x00, 0x02,
	//                           0x00, 0x00, 0x5d, 0x37, 0x12, 0x82, 0x3b, 0xde,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	//                           0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
	send([]byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00, byte(authSendSeq), byte(authSendSeq >> 8),
		byte(localSID >> 24), byte(localSID >> 16), byte(localSID >> 8), byte(localSID),
		byte(remoteSID >> 24), byte(remoteSID >> 16), byte(remoteSID >> 8), byte(remoteSID),
		0x00, 0x00, 0x00, 0x30, 0x01, magic, 0x00, byte(authInnerSendSeq),
		byte(authInnerSendSeq >> 8), 0x00, authID[0], authID[1], authID[2], authID[3], authID[4], authID[5],
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	reauthSent = true
	authSendSeq++
	authInnerSendSeq++
	lastReauthAt = time.Now()
}

func sendDisconnect() {
	send([]byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00, byte(sendSeq), byte(sendSeq >> 8),
		byte(localSID >> 24), byte(localSID >> 16), byte(localSID >> 8), byte(localSID),
		byte(remoteSID >> 24), byte(remoteSID >> 16), byte(remoteSID >> 8), byte(remoteSID),
		0x00, 0x00, 0x00, 0x30, 0x01, 0x01, 0x00, byte(authInnerSendSeq),
		byte(authInnerSendSeq >> 8), 0x00, authID[0], authID[1], authID[2], authID[3], authID[4], authID[5],
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	send([]byte{0x10, 0x00, 0x00, 0x00, 0x05, 0x00, 0x00, 0x00,
		byte(localSID >> 24), byte(localSID >> 16), byte(localSID >> 8), byte(localSID),
		byte(remoteSID >> 24), byte(remoteSID >> 16), byte(remoteSID >> 8), byte(remoteSID)})
}

func sendRequestSerialAndAudio() {
	log.Print("requesting serial and audio stream")
	send([]byte{0x90, 0x00, 0x00, 0x00, 0x00, 0x00, byte(authSendSeq), byte(authSendSeq >> 8),
		byte(localSID >> 24), byte(localSID >> 16), byte(localSID >> 8), byte(localSID), byte(remoteSID >> 24), byte(remoteSID >> 16), byte(remoteSID >> 8), byte(remoteSID),
		0x00, 0x00, 0x00, 0x80, 0x01, 0x03, 0x00, byte(authInnerSendSeq),
		byte(authInnerSendSeq >> 8), 0x00, authID[0], authID[1], authID[2], authID[3], authID[4], authID[5],
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10,
		0x80, 0x00, 0x00, 0x90, 0xc7, 0x0e, 0x86, 0x01, // The last 5 bytes from this row can be acquired from a reply starting with 0xa8 or 0x90
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x49, 0x43, 0x2d, 0x37, 0x30, 0x35, 0x00, 0x00, // IC-705 in plain text
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x2b, 0x3f, 0x55, 0x5c, 0x00, 0x00, 0x00, 0x00, // username: beer
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x01, 0x01, 0x04, 0x04, 0x00, 0x00, 0xbb, 0x80,
		0x00, 0x00, 0xbb, 0x80, 0x00, 0x00, 0xc3, 0x52,
		0x00, 0x00, 0xc3, 0x53, 0x00, 0x00, 0x00, 0x64,
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
	authSendSeq++
	authInnerSendSeq++
}

func main() {
	log.Init()
	parseArgs()
	setupCloseHandler()

	hostPort := fmt.Sprint(connectAddress, ":", connectPort)
	log.Print("connecting to ", hostPort)
	raddr, err := net.ResolveUDPAddr("udp", hostPort)
	if err != nil {
		log.Fatal(err)
	}
	laddr := net.UDPAddr{
		Port: int(connectPort),
	}
	conn, err = net.DialUDP("udp", &laddr, raddr)
	if err != nil {
		log.Fatal(err)
	}

	localSID = uint32(time.Now().Unix())
	log.Debugf("using session id %.8x", localSID)

	sendPkt3()
	sendSeq = 1
	sendPkt7(nil, sendSeq)
	sendSeq = 0
	sendPkt3()

	for {
		// Expecting a Pkt4 answer.
		// Example answer from radio: 0x10, 0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00, 0x8c, 0x7d, 0x45, 0x7a, 0x1d, 0xf6, 0xe9, 0x0b
		r, _ := read()
		if len(r) == 16 && bytes.Equal(r[:8], []byte{0x10, 0x00, 0x00, 0x00, 0x04, 0x00, 0x00, 0x00}) {
			remoteSID = binary.BigEndian.Uint32(r[8:12])
			break
		}
	}

	log.Debugf("got remote session id %.8x", remoteSID)

	authSendSeq = 1
	authInnerSendSeq = 0x50
	sendPkt6()

	for {
		// Expecting a Pkt6 answer.
		r, _ := read()
		if len(r) == 16 && bytes.Equal(r[:8], []byte{0x10, 0x00, 0x00, 0x00, 0x06, 0x00, 0x01, 0x00}) {
			remoteSID = binary.BigEndian.Uint32(r[8:12])
			break
		}
	}

	sendPktLogin()
	sendSeq = 5

	var authOk bool
	var lastPingAt time.Time
	var lastStatusLog time.Time
	var errCount int
	var gotFirstReauthAnswer bool

	_, err = rand.Read(randIDByteForPktSeven[:])
	if err != nil {
		log.Fatal(err)
	}

	for {
		r, err := read()
		if err != nil {
			errCount++
			if errCount > 5 {
				log.Fatal("timeout")
			}
			log.Error("stream break detected")
		}
		errCount = 0

		// Example success auth packet: 0x60, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00,
		//                              0xe6, 0xb2, 0x7b, 0x7b, 0xbb, 0x41, 0x3f, 0x2b,
		//                              0x00, 0x00, 0x00, 0x50, 0x02, 0x00, 0x00, 0x00,
		//                              0x00, 0x00, 0x5d, 0x37, 0x12, 0x82, 0x3b, 0xde,
		//                              0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		//                              0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		//                              0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		//                              0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		//                              0x46, 0x54, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00,
		//                              0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		//                              0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		//                              0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00
		if !authOk {
			if len(r) == 96 && bytes.Equal(r[:8], []byte{0x60, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00}) {
				if bytes.Equal(r[48:52], []byte{0xff, 0xff, 0xff, 0xfe}) {
					log.Fatal("invalid user/password")
				} else {
					authOk = true
					copy(authID[:], r[26:32])
					log.Print("auth ok")

					sendPktReauth()
					time.AfterFunc(time.Second*3, sendRequestSerialAndAudio)
				}
			}
			continue
		}

		if len(r) == 21 && bytes.Equal(r[1:6], []byte{0x00, 0x00, 0x00, 0x07, 0x00}) {
			gotSeq := binary.LittleEndian.Uint16(r[6:8])
			if r[16] == 0x00 { // This is a pkt7 request from the radio.
				// Replying to the radio.
				// Example request from radio: 0x00, 0x00, 0x00, 0x00, 0x07, 0x00, 0x1c, 0x0e, 0xe4, 0x35, 0xdd, 0x72, 0xbe, 0xd9, 0xf2, 0x63, 0x00, 0x57, 0x2b, 0x12, 0x00
				// Example answer from PC:     0x15, 0x00, 0x00, 0x00, 0x07, 0x00, 0x1c, 0x0e, 0xbe, 0xd9, 0xf2, 0x63, 0xe4, 0x35, 0xdd, 0x72, 0x01, 0x57, 0x2b, 0x12, 0x00
				sendPkt7(r[17:21], gotSeq)
			} else {
				if expectedPkt7ReplySeq != gotSeq {
					var missingPkts int
					if gotSeq > expectedPkt7ReplySeq {
						missingPkts = int(gotSeq) - int(expectedPkt7ReplySeq)
					} else {
						missingPkts = int(gotSeq) + 65536 - int(expectedPkt7ReplySeq)
					}
					if missingPkts < 1000 {
						log.Error("lost ", missingPkts, " packets ", gotSeq, " ", expectedPkt7ReplySeq)
					}
				}
			}
		}
		if len(r) == 16 && bytes.Equal(r[:6], []byte{0x10, 0x00, 0x00, 0x00, 0x00, 0x00}) {
			// Replying to the radio.
			// Example request from radio: 0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x13, 0x00, 0xe4, 0x35, 0xdd, 0x72, 0xbe, 0xd9, 0xf2, 0x63
			// Example answer from PC:     0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x13, 0x00, 0xbe, 0xd9, 0xf2, 0x63, 0xe4, 0x35, 0xdd, 0x72
			gotSeq := binary.LittleEndian.Uint16(r[6:8])
			send([]byte{0x10, 0x00, 0x00, 0x00, 0x00, 0x00, byte(gotSeq), byte(gotSeq >> 8), byte(localSID >> 24), byte(localSID >> 16), byte(localSID >> 8), byte(localSID), byte(remoteSID >> 24), byte(remoteSID >> 16), byte(remoteSID >> 8), byte(remoteSID)})
		}
		// if len(r) == 144 && bytes.Equal(r[:6], []byte{0x90, 0x00, 0x00, 0x00, 0x00, 0x00}) {
		// }
		if !gotFirstReauthAnswer && len(r) == 64 && bytes.Equal(r[:6], []byte{0x40, 0x00, 0x00, 0x00, 0x00, 0x00}) { // TODO
			gotFirstReauthAnswer = true
		}
		if len(r) == 80 && bytes.Equal(r[:6], []byte{0x50, 0x00, 0x00, 0x00, 0x00, 0x00}) && bytes.Equal(r[48:51], []byte{0xff, 0xff, 0xff}) {
			// Example answer from radio: 0x50, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03, 0x00,
			//							  0x86, 0x1f, 0x2f, 0xcc, 0x03, 0x03, 0x89, 0x29,
			//							  0x00, 0x00, 0x00, 0x40, 0x02, 0x03, 0x00, 0x52,
			//							  0x00, 0x00, 0xf8, 0xad, 0x06, 0x8d, 0xda, 0x7b,
			//							  0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x10,
			//							  0x80, 0x00, 0x00, 0x90, 0xc7, 0x0e, 0x86, 0x01,
			//							  0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00,
			//							  0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			//							  0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			//							  0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00

			log.Error("reauth failed")
			sendDisconnect()
			os.Exit(1)
		}

		if time.Since(lastPingAt) >= 100*time.Millisecond {
			sendPkt7(nil, sendSeq)
			sendPkt3()
			sendSeq++
			lastPingAt = time.Now()

			if authOk && time.Since(lastReauthAt) >= 60*time.Second {
				sendPktReauth()
			}

			if time.Since(lastStatusLog) >= 10*time.Second {
				log.Print("still connected")
				lastStatusLog = time.Now()
			}
		}
	}
}
