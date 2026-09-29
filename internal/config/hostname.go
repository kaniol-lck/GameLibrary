package config

import "os"

// Hostname returns the machine's host name, or "" when it cannot be determined.
//
// The machine identity is deliberately derived at runtime rather than stored in
// config.json: that file is shared by every client on the NAS, so a persisted
// name would mean whichever client saved last renamed all the others.
func Hostname() string {
	host, err := os.Hostname()
	if err != nil {
		return ""
	}
	return host
}
