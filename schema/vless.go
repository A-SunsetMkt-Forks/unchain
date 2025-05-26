package schema

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/google/uuid"
)

type ProtoVLESS struct {
	userID      uuid.UUID
	DstProtocol string //tcp or udp
	dstHost     string
	dstHostType string //ipv6 or ipv4,domain
	dstPort     uint16
	Version     byte
	payload     []byte
}

func (p ProtoTrojan) AuthUser(password string) (isOk bool) {
	sha224Hash := sha256.New224()
	sha224Hash.Write([]byte(password))
	sha224Sum := sha224Hash.Sum(nil) //28 bytes
	hexSha224Bytes := []byte(fmt.Sprintf("%x", sha224Sum))
	return bytes.Equal(p.sha224password, hexSha224Bytes)
}

func (h ProtoVLESS) UUID() string {
	return h.userID.String()
}

func (h ProtoVLESS) DataUdp() []byte {
	allData := make([]byte, 0)
	chunk := h.payload
	for index := 0; index < len(chunk); {
		if index+2 > len(chunk) {
			fmt.Println("Incomplete length buffer")
			return nil
		}
		lengthBuffer := chunk[index : index+2]
		udpPacketLength := binary.BigEndian.Uint16(lengthBuffer)
		if index+2+int(udpPacketLength) > len(chunk) {
			fmt.Println("Incomplete UDP packet")
			return nil
		}
		udpData := chunk[index+2 : index+2+int(udpPacketLength)]
		index = index + 2 + int(udpPacketLength)
		allData = append(allData, udpData...)
	}
	return allData
}
func (h ProtoVLESS) DataTcp() []byte {
	return h.payload
}

func (h ProtoVLESS) AddrUdp() *net.UDPAddr {
	return &net.UDPAddr{IP: h.HostIP(), Port: int(h.dstPort)}
}
func (h ProtoVLESS) HostIP() net.IP {
	ip := net.ParseIP(h.dstHost)
	if ip == nil {
		ips, err := net.LookupIP(h.dstHost)
		if err != nil {
			h.Logger().Error("failed to resolve domain", "err", err.Error())
			return net.IPv4zero
		}
		if len(ips) == 0 {
			return net.IPv4zero
		}
		return ips[0]
	}
	return ip
}

func (h ProtoVLESS) HostPort() string {
	return net.JoinHostPort(h.dstHost, fmt.Sprintf("%d", h.dstPort))
}
func (h ProtoVLESS) Logger() *slog.Logger {
	return slog.With("userID", h.userID.String(), "network", h.DstProtocol, "addr", h.HostPort())
}

// VLESSParse https://xtls.github.io/development/protocols/vless.html
func VLESSParse(buf []byte) (*ProtoVLESS, error) {
	payload := &ProtoVLESS{
		userID:      uuid.Nil,
		DstProtocol: "",
		dstHost:     "",
		dstPort:     0,
		Version:     0,
		payload:     nil,
	}

	if len(buf) < 24 {
		return payload, errors.New("invalid payload length")
	}

	payload.Version = buf[0]
	payload.userID = uuid.Must(uuid.FromBytes(buf[1:17]))
	extraInfoProtoBufLen := buf[17]

	command := buf[18+extraInfoProtoBufLen]
	switch command {
	case 1:
		payload.DstProtocol = "tcp"
	case 2:
		payload.DstProtocol = "udp"
	default:
		return payload, fmt.Errorf("command %d is not supported, command 01-tcp, 02-udp, 03-mux", command)
	}

	portIndex := 18 + extraInfoProtoBufLen + 1
	payload.dstPort = binary.BigEndian.Uint16(buf[portIndex : portIndex+2])

	addressIndex := portIndex + 2
	addressType := buf[addressIndex]
	addressValueIndex := addressIndex + 1

	switch addressType {
	case 1: // IPv4
		if len(buf) < int(addressValueIndex+net.IPv4len) {
			return nil, fmt.Errorf("invalid IPv4 address length")
		}
		payload.dstHost = net.IP(buf[addressValueIndex : addressValueIndex+net.IPv4len]).String()
		payload.payload = buf[addressValueIndex+net.IPv4len:]
		payload.dstHostType = "ipv4"
	case 2: // domain
		addressLength := buf[addressValueIndex]
		addressValueIndex++
		if len(buf) < int(addressValueIndex)+int(addressLength) {
			return nil, fmt.Errorf("invalid domain address length")
		}
		payload.dstHost = string(buf[addressValueIndex : int(addressValueIndex)+int(addressLength)])
		payload.payload = buf[int(addressValueIndex)+int(addressLength):]
		payload.dstHostType = "domain"

	case 3: // IPv6
		if len(buf) < int(addressValueIndex+net.IPv6len) {
			return nil, fmt.Errorf("invalid IPv6 address length")
		}
		payload.dstHost = net.IP(buf[addressValueIndex : addressValueIndex+net.IPv6len]).String()
		payload.payload = buf[addressValueIndex+net.IPv6len:]
		payload.dstHostType = "ipv6"
	default:
		return nil, fmt.Errorf("addressType %d is not supported", addressType)
	}

	return payload, nil
}
