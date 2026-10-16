package checker

import (
	"net"
	"time"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
)

func TCPCheck(
	node model.NodeInfo,
	timeout time.Duration,
) (time.Duration, error) {

	start := time.Now()

	conn, err := net.DialTimeout(
		"tcp",
		net.JoinHostPort(
			node.Host,
			node.Port,
		),
		timeout,
	)

	if err != nil {

		return 0, err

	}

	conn.Close()

	return time.Since(start), nil

}
