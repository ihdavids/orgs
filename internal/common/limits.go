package common

import "time"

const (
	// Maximum message size allowed from peer.
	MaxMessageSize = 1024*64
	PongWait       = 60 * time.Second
)
