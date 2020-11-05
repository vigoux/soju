package msgstore

import (
	"time"

	"gopkg.in/irc.v3"

	"git.sr.ht/~emersion/soju/database"
)

// Store is a per-user store for IRC messages.
//
// A Store is not safe for concurrent use from multiple goroutines.
type Store interface {
	Close() error
	// LastMsgID queries the last message ID for the given network, entity and
	// date. The message ID returned may not refer to a valid message, but can
	// be used in history queries.
	LastMsgID(network *database.Network, entity string, t time.Time) (string, error)
	LoadBeforeTime(network *database.Network, entity string, t time.Time, limit int) ([]*irc.Message, error)
	LoadAfterTime(network *database.Network, entity string, t time.Time, limit int) ([]*irc.Message, error)
	LoadLatestID(network *database.Network, entity, msgID string, limit int) ([]*irc.Message, error)
	Append(network *database.Network, entity string, msg *irc.Message) (msgID string, err error)
}
